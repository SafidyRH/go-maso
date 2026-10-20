package validation

import (
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strings"
)

func UUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}

	compact := strings.ReplaceAll(value, "-", "")
	_, err := hex.DecodeString(compact)
	return err == nil
}

func HTTPURL(value string, required bool) error {
	if value == "" {
		if required {
			return errors.New("URL is required")
		}
		return nil
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return errors.New("URL must be valid")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("URL scheme must be http or https")
	}

	if parsed.Host == "" || parsed.Hostname() == "" {
		return errors.New("URL host is required")
	}

	if parsed.User != nil {
		return errors.New("URL credentials are not allowed")
	}

	if strings.Contains(parsed.Hostname(), "%") {
		return errors.New("IPv6 zone identifiers are not allowed")
	}

	if port := parsed.Port(); port != "" {
		if _, err := net.LookupPort("tcp", port); err != nil {
			return errors.New("URL port is invalid")
		}
	}

	return nil
}
