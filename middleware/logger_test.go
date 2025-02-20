// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	// Note: Just for the test coverage, not a real test.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := Logger()(func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	})

	// Status 2xx
	h(c)

	// Status 3xx
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	h = Logger()(func(c echo.Context) error {
		return c.String(http.StatusTemporaryRedirect, "test")
	})
	h(c)

	// Status 4xx
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	h = Logger()(func(c echo.Context) error {
		return c.String(http.StatusNotFound, "test")
	})
	h(c)

	// Status 5xx with empty path
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	h = Logger()(func(c echo.Context) error {
		return errors.New("error")
	})
	h(c)
}

func TestLoggerIPAddress(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	buf := new(bytes.Buffer)
	e.Logger.SetOutput(buf)
	ip := "127.0.0.1"
	h := Logger()(func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	})

	// With X-Real-IP
	req.Header.Add(echo.HeaderXRealIP, ip)
	h(c)
	assert.Contains(t, buf.String(), ip)

	// With X-Forwarded-For
	buf.Reset()
	req.Header.Del(echo.HeaderXRealIP)
	req.Header.Add(echo.HeaderXForwardedFor, ip)
	h(c)
	assert.Contains(t, buf.String(), ip)

	buf.Reset()
	h(c)
	assert.Contains(t, buf.String(), ip)
}

func TestLoggerTemplate(t *testing.T) {
	buf := new(bytes.Buffer)

	e := echo.New()
	e.Use(LoggerWithConfig(LoggerConfig{
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}","host":"${host}","user_agent":"${user_agent}",` +
			`"method":"${method}","uri":"${uri}","status":${status}, "latency":${latency},` +
			`"latency_human":"${latency_human}","bytes_in":${bytes_in}, "path":"${path}", "route":"${route}", "referer":"${referer}",` +
			`"bytes_out":${bytes_out},"ch":"${header:X-Custom-Header}", "protocol":"${protocol}"` +
			`"us":"${query:username}", "cf":"${form:username}", "session":"${cookie:session}"}` + "\n",
		Output: buf,
	}))

	e.GET("/users/:id", func(c echo.Context) error {
		return c.String(http.StatusOK, "Header Logged")
	})

	req := httptest.NewRequest(http.MethodGet, "/users/1?username=apagano-param&password=secret", nil)
	req.RequestURI = "/"
	req.Header.Add(echo.HeaderXRealIP, "127.0.0.1")
	req.Header.Add("Referer", "google.com")
	req.Header.Add("User-Agent", "echo-tests-agent")
	req.Header.Add("X-Custom-Header", "AAA-CUSTOM-VALUE")
	req.Header.Add("X-Request-ID", "6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	req.Header.Add("Cookie", "_ga=GA1.2.000000000.0000000000; session=ac08034cd216a647fc2eb62f2bcf7b810")
	req.Form = url.Values{
		"username": []string{"apagano-form"},
		"password": []string{"secret-form"},
	}

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	cases := map[string]bool{
		"apagano-param":                        true,
		"apagano-form":                         true,
		"AAA-CUSTOM-VALUE":                     true,
		"BBB-CUSTOM-VALUE":                     false,
		"secret-form":                          false,
		"hexvalue":                             false,
		"GET":                                  true,
		"127.0.0.1":                            true,
		"\"path\":\"/users/1\"":                true,
		"\"route\":\"/users/:id\"":             true,
		"\"uri\":\"/\"":                        true,
		"\"status\":200":                       true,
		"\"bytes_in\":0":                       true,
		"google.com":                           true,
		"echo-tests-agent":                     true,
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8": true,
		"ac08034cd216a647fc2eb62f2bcf7b810":    true,
	}

	for token, present := range cases {
		assert.True(t, strings.Contains(buf.String(), token) == present, "Case: "+token)
	}
}

func TestLoggerCustomTimestamp(t *testing.T) {
	buf := new(bytes.Buffer)
	customTimeFormat := "2006-01-02 15:04:05.00000"
	e := echo.New()
	e.Use(LoggerWithConfig(LoggerConfig{
		Format: `{"time":"${time_custom}","id":"${id}","remote_ip":"${remote_ip}","host":"${host}","user_agent":"${user_agent}",` +
			`"method":"${method}","uri":"${uri}","status":${status}, "latency":${latency},` +
			`"latency_human":"${latency_human}","bytes_in":${bytes_in}, "path":"${path}", "referer":"${referer}",` +
			`"bytes_out":${bytes_out},"ch":"${header:X-Custom-Header}",` +
			`"us":"${query:username}", "cf":"${form:username}", "session":"${cookie:session}"}` + "\n",
		CustomTimeFormat: customTimeFormat,
		Output:           buf,
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "custom time stamp test")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var objs map[string]*json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &objs); err != nil {
		panic(err)
	}
	loggedTime := *(*string)(unsafe.Pointer(objs["time"]))
	_, err := time.Parse(customTimeFormat, loggedTime)
	assert.Error(t, err)
}

func TestLoggerCustomTagFunc(t *testing.T) {
	e := echo.New()
	buf := new(bytes.Buffer)
	e.Use(LoggerWithConfig(LoggerConfig{
		Format: `{"method":"${method}",${custom}}` + "\n",
		CustomTagFunc: func(c echo.Context, buf *bytes.Buffer) (int, error) {
			return buf.WriteString(`"tag":"my-value"`)
		},
		Output: buf,
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "custom time stamp test")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, `{"method":"GET","tag":"my-value"}`+"\n", buf.String())
}

func BenchmarkLoggerWithConfig_withoutMapFields(b *testing.B) {
	e := echo.New()

	buf := new(bytes.Buffer)
	mw := LoggerWithConfig(LoggerConfig{
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}","host":"${host}","user_agent":"${user_agent}",` +
			`"method":"${method}","uri":"${uri}","status":${status}, "latency":${latency},` +
			`"latency_human":"${latency_human}","bytes_in":${bytes_in}, "path":"${path}", "referer":"${referer}",` +
			`"bytes_out":${bytes_out}, "protocol":"${protocol}"}` + "\n",
		Output: buf,
	})(func(c echo.Context) error {
		c.Request().Header.Set(echo.HeaderXRequestID, "123")
		c.FormValue("to force parse form")
		return c.String(http.StatusTeapot, "OK")
	})

	f := make(url.Values)
	f.Set("csrf", "token")
	f.Add("multiple", "1")
	f.Add("multiple", "2")
	req := httptest.NewRequest(http.MethodPost, "/test?lang=en&checked=1&checked=2", strings.NewReader(f.Encode()))
	req.Header.Set("Referer", "https://echo.labstack.com/")
	req.Header.Set("User-Agent", "curl/7.68.0")
	req.Header.Add(echo.HeaderContentType, echo.MIMEApplicationForm)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		mw(c)
		buf.Reset()
	}
}

func BenchmarkLoggerWithConfig_withMapFields(b *testing.B) {
	e := echo.New()

	buf := new(bytes.Buffer)
	mw := LoggerWithConfig(LoggerConfig{
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}","host":"${host}","user_agent":"${user_agent}",` +
			`"method":"${method}","uri":"${uri}","status":${status}, "latency":${latency},` +
			`"latency_human":"${latency_human}","bytes_in":${bytes_in}, "path":"${path}", "referer":"${referer}",` +
			`"bytes_out":${bytes_out},"ch":"${header:X-Custom-Header}", "protocol":"${protocol}"` +
			`"us":"${query:username}", "cf":"${form:csrf}", "Referer2":"${header:Referer}"}` + "\n",
		Output: buf,
	})(func(c echo.Context) error {
		c.Request().Header.Set(echo.HeaderXRequestID, "123")
		c.FormValue("to force parse form")
		return c.String(http.StatusTeapot, "OK")
	})

	f := make(url.Values)
	f.Set("csrf", "token")
	f.Add("multiple", "1")
	f.Add("multiple", "2")
	req := httptest.NewRequest(http.MethodPost, "/test?lang=en&checked=1&checked=2", strings.NewReader(f.Encode()))
	req.Header.Set("Referer", "https://echo.labstack.com/")
	req.Header.Set("User-Agent", "curl/7.68.0")
	req.Header.Add(echo.HeaderContentType, echo.MIMEApplicationForm)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		mw(c)
		buf.Reset()
	}
}

func TestLoggerTemplateWithTimeUnixMilli(t *testing.T) {
	buf := new(bytes.Buffer)

	e := echo.New()
	e.Use(LoggerWithConfig(LoggerConfig{
		Format: `${time_unix_milli}`,
		Output: buf,
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	unixMillis, err := strconv.ParseInt(buf.String(), 10, 64)
	assert.NoError(t, err)
	assert.WithinDuration(t, time.Unix(unixMillis/1000, 0), time.Now(), 3*time.Second)
}

func TestLoggerTemplateWithTimeUnixMicro(t *testing.T) {
	buf := new(bytes.Buffer)

	e := echo.New()
	e.Use(LoggerWithConfig(LoggerConfig{
		Format: `${time_unix_micro}`,
		Output: buf,
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	unixMicros, err := strconv.ParseInt(buf.String(), 10, 64)
	assert.NoError(t, err)
	assert.WithinDuration(t, time.Unix(unixMicros/1000000, 0), time.Now(), 3*time.Second)
}

/*
func TestLoggerCustomTagFuncError(t *testing.T) {
	// Create a new Echo instance
	e := echo.New()

	// Define a custom tag function for testing
	customTagFunc := func(c echo.Context, buf *bytes.Buffer) (int, error) {
		return buf.WriteString(`"custom":"custom tag error"`)
	}

	// Configure the Logger middleware with the custom tag function
	loggerConfig := LoggerConfig{
		Format:        `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}",` +
			`"host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}",` +
			`"status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}"` +
			`,"bytes_in":${bytes_in},"bytes_out":${bytes_out},"custom":${custom}}`,
		CustomTagFunc: customTagFunc,
	}

	// Apply the logger middleware with the custom config
	e.Use(LoggerWithConfig(loggerConfig))

	// Create a request and a response recorder to capture the output
	req := httptest.NewRequest(echo.GET, "/", nil)
	rec := httptest.NewRecorder()

	// Create a context for the request
	c := e.NewContext(req, rec)

	// Call the handler function (for testing, no actual handler)
	handler := func(c echo.Context) error {
		return c.String(200, "OK")
	}

	// Wrap the handler with the middleware
	if err := LoggerWithConfig(loggerConfig)(handler)(c); err != nil {
		t.Fatal(err)
	}

	// Now, we will check if the log output contains the expected values

	// Capture the output from the response recorder
	logOutput := rec.Body.String()

	// Assert that the custom tag "custom tag error" is present in the output
	assert.Contains(t, logOutput, `"custom":"custom tag error"`)

	// Optionally, assert other key fields to make sure the log structure is correct
	assert.Contains(t, logOutput, `"method":"GET"`)
	assert.Contains(t, logOutput, `"status":200`)
	assert.Contains(t, logOutput, `"uri":"/"`)
	assert.Contains(t, logOutput, `"latency_human"`)
}



/*

func TestLoggerWithConfig_CustomTagError(t *testing.T) {
	// Crea un contesto Echo per simulare una richiesta HTTP
	e := echo.New()

	// Crea una richiesta di esempio
	req := httptest.NewRequest(echo.GET, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Configurazione del logger con un CustomTagFunc che genera un errore
	config := middleware.LoggerConfig{
		Format: `{"custom":"${custom}"}`,
		CustomTagFunc: func(c echo.Context, buf *bytes.Buffer) (int, error) {
			return 0, errors.New("custom tag error") // Simula un errore nel tag personalizzato
		},
		Output: rec,
	}

	// Crea il middleware con la configurazione personalizzata
	logger := middleware.LoggerWithConfig(config)

	// Usa il middleware con una funzione next che non fa nulla
	err := logger(func(c echo.Context) error {
		return nil
	})(c)

	// Assicurati che l'errore nel middleware venga catturato
	assert.Error(t, err)  // Ora verifichiamo che l'errore non sia nil, per far fallire il test se presente

	// Verifica che la risposta contenga il tag "custom" ma con un valore vuoto o errore
	assert.Contains(t, rec.Body.String(), `"custom":""`) // Questo dovrebbe fallire perché non ci sarà un valore vuoto

	// Se preferisci testare l'errore nel body, puoi anche verificare se l'errore è stato scritto da qualche parte
	// Ad esempio, puoi aggiungere una verifica sulla presenza dell'errore nel corpo della risposta
}



/*
// Funzione di test per LoggerWithConfig
func TestLoggerWithConfig_CustomTagError(t *testing.T) {
	// Crea un contesto Echo per simulare una richiesta HTTP
	e := echo.New()

	// Crea una richiesta di esempio
	req := httptest.NewRequest(echo.GET, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Configurazione del logger con un CustomTagFunc che genera un errore
	config := middleware.LoggerConfig{
		Format:          `{"custom":"${custom}"}`,
		CustomTagFunc: func(c echo.Context, buf *bytes.Buffer) (int, error) {
			return 0, errors.New("custom tag error") // Simula un errore nel tag personalizzato
		},
		Output: rec,
	}

	// Crea il middleware con la configurazione personalizzata
	logger := middleware.LoggerWithConfig(config)

	// Usa il middleware con una funzione next che non fa nulla
	err := logger(func(c echo.Context) error {
		return nil
	})(c)

	// Verifica che non ci siano errori e che la risposta contenga il tag custom vuoto o un valore di errore
	assert.NoError(t, err)
	assert.Contains(t, rec.Body.String(), `"custom":""`) // Assicurati che il tag custom sia vuoto o assente

	// Se preferisci testare specificamente il caso di errore, puoi controllare anche se l'errore è stato scritto da qualche parte
	// Ad esempio, potresti verificare se il logger ha registrato l'errore nel body della risposta
}
*/

func TestLoggerCustomTagFunc2(t *testing.T) {
	e := echo.New()
	buf := new(bytes.Buffer)
	e.Use(LoggerWithConfig(LoggerConfig{
		Format: `{"protocol":"${protocol}",${custom}}` + "\n",
		CustomTagFunc: func(c echo.Context, buf *bytes.Buffer) (int, error) {
			return buf.WriteString(`"tag":"my-value"`)
		},
		Output: buf,
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "custom time stamp test")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, `{"protocol":"HTTP/1.1","tag":"my-value"}`+"\n", buf.String())
}

func TestLoggerWithCustomHeader(t *testing.T) {
	e := echo.New()
	buf := new(bytes.Buffer)
	config := LoggerConfig{
		Format: `{"custom_header":"${header:X-Custom-Header}"}\n`,
		Output: buf,
	}
	middleware := LoggerWithConfig(config)
	h := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Custom-Header", "test-value")
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)

	// Act
	_ = h(c)

	// Assert
	logOutput := buf.String()
	assert.Contains(t, logOutput, `"custom_header":"test-value"`)
}
