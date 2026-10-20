package validation

import "testing"

func TestHTTPURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "https", value: "https://example.com/health"},
		{name: "http with port", value: "http://example.com:8080/health"},
		{name: "missing scheme", value: "example.com", wantErr: true},
		{name: "unsupported scheme", value: "file:///etc/passwd", wantErr: true},
		{name: "credentials", value: "https://user:secret@example.com", wantErr: true},
		{name: "invalid port", value: "https://example.com:not-a-port", wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := HTTPURL(test.value, true)
			if (err != nil) != test.wantErr {
				t.Fatalf("HTTPURL(%q) error = %v, wantErr %v", test.value, err, test.wantErr)
			}
		})
	}
}

func TestUUID(t *testing.T) {
	t.Parallel()

	if !UUID("123e4567-e89b-12d3-a456-426614174000") {
		t.Fatal("UUID rejected a valid UUID")
	}
	for _, value := range []string{"", "not-a-uuid", "123e4567-e89b-12d3-a456-42661417400z"} {
		if UUID(value) {
			t.Fatalf("UUID(%q) = true, want false", value)
		}
	}
}
