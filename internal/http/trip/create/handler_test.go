package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/http/trip/create"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

const nullJSON = "null"

const jsonMediaType = "application/json"

const validBody = `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":59.9,"longitude":30.3},"end_point":{"latitude":59.8,"longitude":30.4},"price":1450}`

func TestCreate(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{{"success", nil, 201, ""}, {"busy", trip.ErrDriverBusy, 409, "driver_busy"}, {"failure", errors.New("secret SQL password"), 500, "internal_error"}} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			u := NewMockusecase(ctrl)
			id := uuid.New()
			value := model.Trip{ID: id, Status: trip.Active, StartedAt: time.Now().UTC()}
			u.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, i model.CreateInput) (model.Trip, error) {
				if i.Price != 1450 || i.StartPoint.Latitude != 59.9 || i.UserID.String() != "5cb72c04-7650-45c9-a79b-bcdba0631e0c" {
					t.Fatalf("wrong input: %+v", i)
				}
				return value, tt.err
			})
			r := httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(validBody))
			r.Header.Set("Content-Type", jsonMediaType)
			w := httptest.NewRecorder()
			create.New(u).CreateTrip(w, r, api.CreateTripParams{})
			if w.Code != tt.status {
				t.Fatalf("status=%d,body=%s", w.Code, w.Body)
			}
			if tt.err == nil {
				var got api.Trip
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Id != id || w.Header().Get("Location") != "/api/v1/trips/"+id.String() {
					t.Fatalf("response=%s", w.Body)
				}
				return
			}
			var problem api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != tt.code || w.Header().Get("Content-Type") != "application/problem+json" || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("problem=%s", w.Body)
			}
		})
	}
}

func TestInvalidBody(t *testing.T) {
	for _, tt := range []struct{ name, body, media string }{
		{"empty", "", jsonMediaType},
		{"malformed", "{", jsonMediaType},
		{nullJSON, nullJSON, jsonMediaType},
		{"array", "[]", jsonMediaType},
		{"wrong media", validBody, "text/plain"},
		{"missing media", validBody, ""},
		{"second object", validBody + "{}", jsonMediaType},
		{"missing price", strings.Replace(validBody, `,"price":1450`, "", 1), jsonMediaType},
		{"null price", strings.Replace(validBody, `"price":1450`, `"price":null`, 1), jsonMediaType},
		{"unknown", strings.Replace(validBody, `"price":1450`, `"price":1450,"extra":1`, 1), jsonMediaType},
		{"missing coordinate", strings.Replace(validBody, `"latitude":59.9,`, "", 1), jsonMediaType},
		{"null point", strings.Replace(validBody, `{"latitude":59.9,"longitude":30.3}`, nullJSON, 1), jsonMediaType},
		{"bad UUID", strings.Replace(validBody, "5cb72c04-7650-45c9-a79b-bcdba0631e0c", "invalid", 1), jsonMediaType},
		{"zero UUID", strings.Replace(validBody, "5cb72c04-7650-45c9-a79b-bcdba0631e0c", uuid.Nil.String(), 1), jsonMediaType},
		{"negative price", strings.Replace(validBody, `1450`, `-1`, 1), jsonMediaType},
		{"out of range", strings.Replace(validBody, `59.9`, `91`, 1), jsonMediaType},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			u := NewMockusecase(ctrl)
			r := httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", tt.media)
			w := httptest.NewRecorder()
			create.New(u).CreateTrip(w, r, api.CreateTripParams{})
			var p api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if w.Code != 400 || p.Code != "invalid_request" {
				t.Fatalf("status=%d,body=%s", w.Code, w.Body)
			}
		})
	}
}

func TestValidationDetails(t *testing.T) {
	for _, tt := range []struct{ name, body, media, detail string }{
		{"missing", strings.Replace(validBody, `,"price":1450`, "", 1), jsonMediaType, "price: is required"},
		{nullJSON, strings.Replace(validBody, `"price":1450`, `"price":null`, 1), jsonMediaType, "price: must not be null"},
		{"unknown", strings.Replace(validBody, `"price":1450`, `"price":1450,"extra":1`, 1), jsonMediaType, "extra: unknown field"},
		{"unknown sorted", strings.Replace(validBody, `"price":1450`, `"price":1450,"zzz":1,"aaa":2`, 1), jsonMediaType, "aaa: unknown field"},
		{"nested missing", strings.Replace(validBody, `"latitude":59.9,`, "", 1), jsonMediaType, "start_point.latitude: is required"},
		{"nested null", strings.Replace(validBody, `"longitude":30.4`, `"longitude":null`, 1), jsonMediaType, "end_point.longitude: must not be null"},
		{"nested unknown", strings.Replace(validBody, `"longitude":30.4`, `"longitude":30.4,"speed":1`, 1), jsonMediaType, "end_point.speed: unknown field"},
		{"UUID", strings.Replace(validBody, "8860b315-ec86-42eb-a17c-7c163d721ff5", "broken", 1), jsonMediaType, "driver_id: must be a valid UUID"},
		{"price type", strings.Replace(validBody, `"price":1450`, `"price":"text"`, 1), jsonMediaType, "price: must be an integer"},
		{"coordinate type", strings.Replace(validBody, `"latitude":59.9`, `"latitude":"text"`, 1), jsonMediaType, "start_point.latitude: must be a number"},
		{"point type", strings.Replace(validBody, `{"latitude":59.9,"longitude":30.3}`, `[]`, 1), jsonMediaType, "start_point: must be a JSON object"},
		{"price range", strings.Replace(validBody, `"price":1450`, `"price":-1`, 1), jsonMediaType, "price: must be non-negative"},
		{"content type", validBody, "text/plain", "Content-Type: must be application/json"},
		{"empty", "", jsonMediaType, "body: request body is required"},
		{"truncated", "{", jsonMediaType, "body: incomplete JSON body"},
		{"root null", nullJSON, jsonMediaType, "body: must be a JSON object"},
		{"second object", validBody + "{}", jsonMediaType, "body: must contain a single JSON object"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			u := NewMockusecase(ctrl)
			r := httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", tt.media)
			w := httptest.NewRecorder()
			create.New(u).CreateTrip(w, r, api.CreateTripParams{})
			var problem api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if w.Code != 400 || problem.Code != "invalid_request" || problem.Detail == nil || *problem.Detail != tt.detail {
				t.Fatalf("response=%d %s, want %q", w.Code, w.Body, tt.detail)
			}
		})
	}
}

func TestResponseEncodingFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	u := NewMockusecase(ctrl)
	u.EXPECT().Create(gomock.Any(), gomock.Any()).Return(model.Trip{ID: uuid.New(), StartedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}, nil)
	r := httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(validBody))
	r.Header.Set("Content-Type", jsonMediaType)
	w := httptest.NewRecorder()
	create.New(u).CreateTrip(w, r, api.CreateTripParams{})
	var problem api.Problem
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 || problem.Code != "internal_error" || w.Header().Get("Content-Type") != "application/problem+json" || w.Header().Get("Location") != "" || strings.Contains(w.Body.String(), "10000") {
		t.Fatalf("serialization error response=%d %v %s", w.Code, w.Header(), w.Body)
	}
}
