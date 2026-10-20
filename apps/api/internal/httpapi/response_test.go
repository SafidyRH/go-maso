package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"known":"value","other":true}`))
	recorder := httptest.NewRecorder()
	var input struct {
		Known string `json:"known"`
	}

	if err := DecodeJSON(recorder, request, &input); err == nil {
		t.Fatal("DecodeJSON succeeded, want unknown field error")
	}
}

func TestDecodeJSONRejectsSecondObject(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("POST", "/", strings.NewReader(`{} {}`))
	recorder := httptest.NewRecorder()

	if err := DecodeJSON(recorder, request, &struct{}{}); err == nil {
		t.Fatal("DecodeJSON succeeded, want second object error")
	}
}
