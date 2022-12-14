// Package server defines the models and handler functions used on the server side.
package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	questionCount = 4
)

type AppResource struct {
	Client *mongo.Client
}

// NewAppResource creates an AppResource that's connected to the database.
func NewAppResource() *AppResource {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("You must set your 'MONGODB_URI' environmental variable. See\n\t https://www.mongodb.com/docs/drivers/go/current/usage-examples/#environment-variable")
	}
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	rs := AppResource{
		Client: client,
	}
	return &rs
}

// Close disconnects from the database.
func (rs *AppResource) Close() {
	if err := rs.Client.Disconnect(context.TODO()); err != nil {
		panic(err)
	}
}

// Db returns the database we're interested in.
func (rs *AppResource) Db() *mongo.Database {
	return rs.Client.Database("voting")
}

// BranchCtx checks if the branch is either "senate" or "treasury", and sets the context value.
func BranchCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var branch string

		branch = chi.URLParam(r, "branch")
		if branch != "senate" && branch != "treasury" {
			data := map[string]string{
				"message": fmt.Sprintf(`Invalid branch "%s"; must be either "senate" or "treasury"`, branch),
			}
			render.Render(w, r, NewResponseFail(data))
			return
		}

		ctx := context.WithValue(r.Context(), "branch", branch)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// randomize randomizes a slice.
func randomize[T any](x []T) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(x), func(i, j int) {
		x[i], x[j] = x[j], x[i]
	})
}

// GetCandidates renders all the candidates.
// The order of the candidates is randomized.
func (rs *AppResource) GetCandidates(w http.ResponseWriter, r *http.Request) {
	branch := r.Context().Value("branch").(string)
	collection := rs.Db().Collection(branch)

	cur, err := collection.Find(r.Context(), bson.D{})
	if err != nil {
		render.Render(w, r, NewErrorResponse("Could not get cursor from db"))
		return
	}
	defer cur.Close(r.Context())

	var candidates []Candidate
	for cur.Next(r.Context()) {
		candidate := Candidate{}
		err := cur.Decode(&candidate)
		if err != nil {
			render.Render(w, r, NewErrorResponse("Could not decode into candidate"))
			return
		}

		candidates = append(candidates, candidate)
	}
	randomize(candidates)
	data := map[string]any{
		"candidates": candidates,
	}
	render.Render(w, r, NewResponseSuccess(data))
}

// PostCandidates takes in a candidate JSON and inserts them into the database.
func (rs *AppResource) PostCandidates(w http.ResponseWriter, r *http.Request) {
	data := &CandidateRequest{}
	err := render.Bind(r, data)
	if errors.Is(err, ErrMissingName) {
		render.Render(w, r, NewResponseFail(map[string]string{"name": err.Error()}))
		return
	}
	if errors.Is(err, ErrMissingAnswers) || errors.Is(err, ErrAnswersTooLong) {
		render.Render(w, r, NewResponseFail(map[string]string{"answers": err.Error()}))
		return
	}

	branch := r.Context().Value("branch").(string)
	collection := rs.Db().Collection(branch)
	_, err = collection.InsertOne(
		r.Context(),
		bson.D{
			{"name", data.Name},
			{"answers", data.Answers},
			{"votes", 0},
		},
	)
	if err != nil {
		render.Render(w, r, NewErrorResponse("There was an error adding the candidate to the database."))
		return
	}

	render.Status(r, http.StatusCreated)
	render.Render(w, r, NewResponseSuccess(nil))
}

// PatchVotes increments a candidate's votes by one.
func (rs *AppResource) PatchVotes(w http.ResponseWriter, r *http.Request) {
	id, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		render.Render(w, r, NewErrorResponse("Could not get ObjectID"))
		return
	}

	branch := r.Context().Value("branch").(string)
	collection := rs.Db().Collection(branch)

	_, err = collection.UpdateOne(
		r.Context(),
		bson.M{
			"_id": id,
		},
		bson.D{
			{"$inc", bson.D{{"votes", 1}}},
		},
	)
	if err != nil {
		render.Render(w, r, NewErrorResponse("Could not increment votes"))
		return
	}
	render.Render(w, r, NewResponseSuccess(nil))
}

// GetAnswers renders everyone's answers grouped by question.
// The order within each question is randomized.
func (rs *AppResource) GetAnswers(w http.ResponseWriter, r *http.Request) {
	branch := r.Context().Value("branch").(string)
	collection := rs.Db().Collection(branch)

	cur, err := collection.Find(r.Context(), bson.D{})
	if err != nil {
		render.Render(w, r, NewErrorResponse("Could not get cursor from db"))
		return
	}
	defer cur.Close(r.Context())

	var answers [questionCount][]Answer
	for cur.Next(r.Context()) {
		candidate := Candidate{}
		err := cur.Decode(&candidate)
		if err != nil {
			render.Render(w, r, NewErrorResponse("Could not decode into candidate"))
			return
		}

		for i := 0; i < questionCount; i++ {
			answer := Answer{
				Id:     candidate.Id,
				Name:   candidate.Name,
				Votes:  candidate.Votes,
				Answer: candidate.Answers[i],
			}
			answers[i] = append(answers[i], answer)
		}
	}
	for i := 0; i < questionCount; i++ {
		randomize(answers[i])
	}

	data := map[string]any{
		"answers": answers,
	}
	render.Render(w, r, NewResponseSuccess(data))
}

// GetLeaderboard renders the list of candidates sorted from most to least votes.
func (rs *AppResource) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	branch := r.Context().Value("branch").(string)
	collection := rs.Db().Collection(branch)

	opts := options.Find().SetSort(bson.D{{"votes", -1}})
	cur, err := collection.Find(r.Context(), bson.D{}, opts)
	if err != nil {
		render.Render(w, r, NewErrorResponse("Could not get cursor from db"))
		return
	}
	defer cur.Close(r.Context())

	var leaderboardEntries []LeaderboardEntry
	for cur.Next(r.Context()) {
		leaderboardEntry := LeaderboardEntry{}
		err := cur.Decode(&leaderboardEntry)
		if err != nil {
			render.Render(w, r, NewErrorResponse("Could not decode into leaderboard entry"))
			return
		}

		leaderboardEntries = append(leaderboardEntries, leaderboardEntry)
	}
	data := map[string]any{
		"leaderboard": leaderboardEntries,
	}
	render.Render(w, r, NewResponseSuccess(data))
}

// GetQuestions renders the list of questions.
func (rs *AppResource) GetQuestions(w http.ResponseWriter, r *http.Request) {
	render.Render(w, r, NewErrorResponse("Not implemented"))
}
