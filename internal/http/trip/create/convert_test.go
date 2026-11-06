package create

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saifullinartem/saifullin_golang/internal/domain/trip"
)

const requestBody = `{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c","driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5","start_point":{"latitude":59.9,"longitude":30.3},"end_point":{"latitude":59.8,"longitude":30.4},"price":1450}`

func TestDecodePreservesCause(t *testing.T) {
	for _, tt := range []struct{ name, body, field string }{
		{"price", strings.Replace(requestBody, `"price":1450`, `"price":"text"`, 1), "price"},
		{"point", strings.Replace(requestBody, `{"latitude":59.9,"longitude":30.3}`, `[]`, 1), "start_point"},
		{"coordinate", strings.Replace(requestBody, `"latitude":59.9`, `"latitude":{}`, 1), "start_point.latitude"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")
			_, err := decodeCreate(r)
			var invalid *trip.ValidationError
			var cause *json.UnmarshalTypeError
			if !errors.As(err, &invalid) || invalid.Field != tt.field || !errors.As(err, &cause) || !errors.Is(err, invalid.Cause) {
				t.Fatalf("field/cause lost: %v", err)
			}
		})
	}
}

func TestDecodePreservesSyntaxCause(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"price":!}`))
	r.Header.Set("Content-Type", "application/json")
	_, err := decodeCreate(r)
	var cause *json.SyntaxError
	if !errors.As(err, &cause) || cause.Offset == 0 {
		t.Fatalf("syntax cause lost: %v", err)
	}
}

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }

func TestDecodePreservesReadCause(t *testing.T) {
	cause := errors.New("body read failed")
	r := httptest.NewRequest("POST", "/", failedReader{cause})
	r.Header.Set("Content-Type", "application/json")
	_, err := decodeCreate(r)
	if !errors.Is(err, cause) {
		t.Fatalf("read cause lost: %v", err)
	}
	r = httptest.NewRequest("POST", "/", strings.NewReader("{"))
	r.Header.Set("Content-Type", "application/json")
	_, err = decodeCreate(r)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("incomplete JSON cause lost: %v", err)
	}
}
