// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package middleware

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestBasicAuth(t *testing.T) {
	handlerCalled := false
	handler := func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "test")
	}

	testValidator := func(username, password string, c echo.Context) (bool, error) {
		if username == "valid-user" && password == "valid-pass" {
			return true, nil
		}
		return false, nil
	}

	middlewareChain := BasicAuth(testValidator)(handler)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(echo.HeaderAuthorization, "Basic "+base64.StdEncoding.EncodeToString([]byte("valid-user:valid-pass")))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := middlewareChain(c)

	assert.NoError(t, err)
	assert.True(t, handlerCalled)
}

func TestBasicAuthWithConfig(t *testing.T) {
	e := echo.New()

	mockValidator := func(u, p string, c echo.Context) (bool, error) {
		if u == "joe" && p == "secret" {
			return true, nil
		}
		return false, nil
	}

	tests := []struct {
		name           string
		authHeader     string
		expectedCode   int
		expectedAuth   string
		skipperResult  bool
		expectedErr    bool
		expectedErrMsg string
		shouldPanic    bool
	}{
		{
			name:         "Valid credentials",
			authHeader:   basic + " " + base64.StdEncoding.EncodeToString([]byte("joe:secret")),
			expectedCode: http.StatusOK,
		},
		{
			name:         "Case-insensitive header scheme",
			authHeader:   strings.ToUpper(basic) + " " + base64.StdEncoding.EncodeToString([]byte("joe:secret")),
			expectedCode: http.StatusOK,
		},
		{
			name:           "Invalid credentials",
			authHeader:     basic + " " + base64.StdEncoding.EncodeToString([]byte("joe:invalid-password")),
			expectedCode:   http.StatusUnauthorized,
			expectedAuth:   basic + ` realm="someRealm"`,
			expectedErr:    true,
			expectedErrMsg: "Unauthorized",
		},
		{
			name:           "Invalid base64 string",
			authHeader:     basic + " invalidString",
			expectedCode:   http.StatusBadRequest,
			expectedErr:    true,
			expectedErrMsg: "Bad Request",
		},
		{
			name:           "Missing Authorization header",
			expectedCode:   http.StatusUnauthorized,
			expectedErr:    true,
			expectedErrMsg: "Unauthorized",
		},
		{
			name:           "Invalid Authorization header",
			authHeader:     base64.StdEncoding.EncodeToString([]byte("invalid")),
			expectedCode:   http.StatusUnauthorized,
			expectedErr:    true,
			expectedErrMsg: "Unauthorized",
		},
		{
			name:          "Skipped Request",
			authHeader:    basic + " " + base64.StdEncoding.EncodeToString([]byte("joe:skip")),
			expectedCode:  http.StatusOK,
			skipperResult: true,
		},
		{
			name:        "Panic when validator is nil",
			shouldPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.shouldPanic {
				assert.Panics(t, func() {
					BasicAuthWithConfig(BasicAuthConfig{})
				})
				return
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			res := httptest.NewRecorder()
			c := e.NewContext(req, res)

			if tt.authHeader != "" {
				req.Header.Set(echo.HeaderAuthorization, tt.authHeader)
			}

			h := BasicAuthWithConfig(BasicAuthConfig{
				Validator: mockValidator,
				Realm:     "someRealm",
				Skipper: func(c echo.Context) bool {
					return tt.skipperResult
				},
			})(func(c echo.Context) error {
				return c.String(http.StatusOK, "test")
			})

			err := h(c)

			if tt.expectedErr {
				var he *echo.HTTPError
				errors.As(err, &he)
				assert.Equal(t, tt.expectedCode, he.Code)
				if tt.expectedAuth != "" {
					assert.Equal(t, tt.expectedAuth, res.Header().Get(echo.HeaderWWWAuthenticate))
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCode, res.Code)
			}
		})
	}
}
