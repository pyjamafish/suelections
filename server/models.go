package server

import (
	"errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"net/http"
)

const (
	AnswerMaxLength = 280
)

type JSendStatus string

const (
	Success JSendStatus = "success"
	Fail    JSendStatus = "fail"
	Error   JSendStatus = "error"
)

type Candidate struct {
	Id      primitive.ObjectID `json:"_id" bson:"_id"`
	Name    string             `json:"name"`
	Votes   int32              `json:"votes"`
	Answers []string           `json:"answers"`
}

type LeaderboardEntry struct {
	Id    primitive.ObjectID `json:"_id" bson:"_id"`
	Name  string             `json:"name"`
	Votes int32              `json:"votes"`
}

type Answer struct {
	Id     primitive.ObjectID `json:"_id" bson:"_id"`
	Name   string             `json:"name"`
	Votes  int32              `json:"votes"`
	Answer string             `json:"answer"`
}

type Response struct {
	Status JSendStatus `json:"status"`
	Data   any         `json:"data"`
}

func NewResponseSuccess(data any) *Response {
	return &Response{Status: Success, Data: data}
}

func NewResponseFail(data any) *Response {
	return &Response{Status: Fail, Data: data}
}

func (response *Response) Render(_ http.ResponseWriter, _ *http.Request) error {
	return nil
}

type ErrorResponse struct {
	Status  JSendStatus `json:"status"`
	Message string      `json:"message"`
}

func NewErrorResponse(message string) *ErrorResponse {
	return &ErrorResponse{Status: Error, Message: message}
}

func (response *ErrorResponse) Render(_ http.ResponseWriter, _ *http.Request) error {
	return nil
}

type CandidateRequest struct {
	Name    string   `json:"name"`
	Answers []string `json:"answers"`
}

var ErrMissingName = errors.New("missing name")
var ErrMissingAnswers = errors.New("missing answers")
var ErrAnswersTooLong = errors.New("one or more answers are too long")

func (request *CandidateRequest) Bind(r *http.Request) error {
	if request.Name == "" {
		return ErrMissingName
	}
	if request.Answers == nil {
		return ErrMissingAnswers
	}
	for _, answer := range request.Answers {
		if len(answer) > AnswerMaxLength {
			return ErrAnswersTooLong
		}
	}

	return nil
}
