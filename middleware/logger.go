// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/color"
	"github.com/valyala/fasttemplate"
)

// LoggerConfig defines the config for Logger middleware.
type LoggerConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper Skipper

	// Tags to construct the logger format.
	//
	// - time_unix
	// - time_unix_milli
	// - time_unix_micro
	// - time_unix_nano
	// - time_rfc3339
	// - time_rfc3339_nano
	// - time_custom
	// - id (Request ID)
	// - remote_ip
	// - uri
	// - host
	// - method
	// - path
	// - route
	// - protocol
	// - referer
	// - user_agent
	// - status
	// - error
	// - latency (In nanoseconds)
	// - latency_human (Human readable)
	// - bytes_in (Bytes received)
	// - bytes_out (Bytes sent)
	// - header:<NAME>
	// - query:<NAME>
	// - form:<NAME>
	// - custom (see CustomTagFunc field)
	//
	// Example "${remote_ip} ${status}"
	//
	// Optional. Default value DefaultLoggerConfig.Format.
	Format string `yaml:"format"`

	// Optional. Default value DefaultLoggerConfig.CustomTimeFormat.
	CustomTimeFormat string `yaml:"custom_time_format"`

	// CustomTagFunc is function called for `${custom}` tag to output user implemented text by writing it to buf.
	// Make sure that outputted text creates valid JSON string with other logged tags.
	// Optional.
	CustomTagFunc func(c echo.Context, buf *bytes.Buffer) (int, error)

	// Output is a writer where logs in JSON format are written.
	// Optional. Default value os.Stdout.
	Output io.Writer

	template *fasttemplate.Template
	colorer  *color.Color
	pool     *sync.Pool
}

// DefaultLoggerConfig is the default Logger middleware config.
var DefaultLoggerConfig = LoggerConfig{
	Skipper: DefaultSkipper,
	Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}",` +
		`"host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}",` +
		`"status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}"` +
		`,"bytes_in":${bytes_in},"bytes_out":${bytes_out}}` + "\n",
	CustomTimeFormat: "2006-01-02 15:04:05.00000",
	colorer:          color.New(),
}

// Logger returns a middleware that logs HTTP requests.
func Logger() echo.MiddlewareFunc {
	return LoggerWithConfig(DefaultLoggerConfig)
}

var loggerWithConfigCoverage = make(map[int]bool)

const loggerWithConfigCoverageTotal = 59

// LoggerWithConfig returns a Logger middleware with config.
// See: `Logger()`.
func LoggerWithConfig(config LoggerConfig) echo.MiddlewareFunc {
	// Defaults
	if config.Skipper == nil {
		loggerWithConfigCoverage[0] = true
		config.Skipper = DefaultLoggerConfig.Skipper
	} else {
		loggerWithConfigCoverage[1] = true
	}
	if config.Format == "" {
		loggerWithConfigCoverage[2] = true
		config.Format = DefaultLoggerConfig.Format
	} else {
		loggerWithConfigCoverage[3] = true
	}
	if config.Output == nil {
		loggerWithConfigCoverage[4] = true
		config.Output = DefaultLoggerConfig.Output
	} else {
		loggerWithConfigCoverage[5] = true
	}

	config.template = fasttemplate.New(config.Format, "${", "}")
	config.colorer = color.New()
	config.colorer.SetOutput(config.Output)
	config.pool = &sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 256))
		},
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			if config.Skipper(c) {
				loggerWithConfigCoverage[6] = true
				return next(c)
			}
			loggerWithConfigCoverage[7] = true

			req := c.Request()
			res := c.Response()
			start := time.Now()
			if err = next(c); err != nil {
				loggerWithConfigCoverage[8] = true
				c.Error(err)
			} else {
				loggerWithConfigCoverage[9] = true
			}
			stop := time.Now()
			buf := config.pool.Get().(*bytes.Buffer)
			buf.Reset()
			defer config.pool.Put(buf)

			if _, err = config.template.ExecuteFunc(buf, func(w io.Writer, tag string) (int, error) {
				switch tag {
				case "custom":
					loggerWithConfigCoverage[10] = true
					if config.CustomTagFunc == nil {
						loggerWithConfigCoverage[11] = true
						return 0, nil
					}
					loggerWithConfigCoverage[12] = true
					return config.CustomTagFunc(c, buf)
				case "time_unix":
					loggerWithConfigCoverage[13] = true
					return buf.WriteString(strconv.FormatInt(time.Now().Unix(), 10))
				case "time_unix_milli":
					// go 1.17 or later, it supports time#UnixMilli()
					loggerWithConfigCoverage[14] = true
					return buf.WriteString(strconv.FormatInt(time.Now().UnixNano()/1000000, 10))
				case "time_unix_micro":
					// go 1.17 or later, it supports time#UnixMicro()
					loggerWithConfigCoverage[15] = true
					return buf.WriteString(strconv.FormatInt(time.Now().UnixNano()/1000, 10))
				case "time_unix_nano":
					loggerWithConfigCoverage[16] = true
					return buf.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10))
				case "time_rfc3339":
					loggerWithConfigCoverage[17] = true
					return buf.WriteString(time.Now().Format(time.RFC3339))
				case "time_rfc3339_nano":
					loggerWithConfigCoverage[18] = true
					return buf.WriteString(time.Now().Format(time.RFC3339Nano))
				case "time_custom":
					loggerWithConfigCoverage[19] = true
					return buf.WriteString(time.Now().Format(config.CustomTimeFormat))
				case "id":
					loggerWithConfigCoverage[20] = true
					id := req.Header.Get(echo.HeaderXRequestID)
					if id == "" {
						loggerWithConfigCoverage[21] = true
						id = res.Header().Get(echo.HeaderXRequestID)
					} else {
						loggerWithConfigCoverage[22] = true
					}
					return buf.WriteString(id)
				case "remote_ip":
					loggerWithConfigCoverage[23] = true
					return buf.WriteString(c.RealIP())
				case "host":
					loggerWithConfigCoverage[24] = true
					return buf.WriteString(req.Host)
				case "uri":
					loggerWithConfigCoverage[25] = true
					return buf.WriteString(req.RequestURI)
				case "method":
					loggerWithConfigCoverage[26] = true
					return buf.WriteString(req.Method)
				case "path":
					loggerWithConfigCoverage[27] = true
					p := req.URL.Path
					if p == "" {
						loggerWithConfigCoverage[28] = true
						p = "/"
					} else {
						loggerWithConfigCoverage[29] = true
					}
					return buf.WriteString(p)
				case "route":
					loggerWithConfigCoverage[30] = true
					return buf.WriteString(c.Path())
				case "protocol":
					loggerWithConfigCoverage[31] = true
					return buf.WriteString(req.Proto)
				case "referer":
					loggerWithConfigCoverage[32] = true
					return buf.WriteString(req.Referer())
				case "user_agent":
					loggerWithConfigCoverage[33] = true
					return buf.WriteString(req.UserAgent())
				case "status":
					loggerWithConfigCoverage[34] = true
					n := res.Status
					s := config.colorer.Green(n)
					switch {
					case n >= 500:
						loggerWithConfigCoverage[35] = true
						s = config.colorer.Red(n)
					case n >= 400:
						loggerWithConfigCoverage[36] = true
						s = config.colorer.Yellow(n)
					case n >= 300:
						loggerWithConfigCoverage[37] = true
						s = config.colorer.Cyan(n)
					default:
						loggerWithConfigCoverage[38] = true
					}
					return buf.WriteString(s)
				case "error":
					loggerWithConfigCoverage[39] = true
					if err != nil {
						// Error may contain invalid JSON e.g. `"`
						loggerWithConfigCoverage[40] = true
						b, _ := json.Marshal(err.Error())
						b = b[1 : len(b)-1]
						return buf.Write(b)
					}
					loggerWithConfigCoverage[41] = true
				case "latency":
					loggerWithConfigCoverage[42] = true
					l := stop.Sub(start)
					return buf.WriteString(strconv.FormatInt(int64(l), 10))
				case "latency_human":
					loggerWithConfigCoverage[43] = true
					return buf.WriteString(stop.Sub(start).String())
				case "bytes_in":
					loggerWithConfigCoverage[44] = true
					cl := req.Header.Get(echo.HeaderContentLength)
					if cl == "" {
						loggerWithConfigCoverage[45] = true
						cl = "0"
					} else {
						loggerWithConfigCoverage[46] = true
					}
					return buf.WriteString(cl)
				case "bytes_out":
					loggerWithConfigCoverage[47] = true
					return buf.WriteString(strconv.FormatInt(res.Size, 10))
				default:
					switch {
					case strings.HasPrefix(tag, "header:"):
						loggerWithConfigCoverage[48] = true
						return buf.Write([]byte(c.Request().Header.Get(tag[7:])))
					case strings.HasPrefix(tag, "query:"):
						loggerWithConfigCoverage[49] = true
						return buf.Write([]byte(c.QueryParam(tag[6:])))
					case strings.HasPrefix(tag, "form:"):
						loggerWithConfigCoverage[50] = true
						return buf.Write([]byte(c.FormValue(tag[5:])))
					case strings.HasPrefix(tag, "cookie:"):
						loggerWithConfigCoverage[51] = true
						cookie, err := c.Cookie(tag[7:])
						if err == nil {
							loggerWithConfigCoverage[52] = true
							return buf.Write([]byte(cookie.Value))
						}
						loggerWithConfigCoverage[53] = true
					default:
						loggerWithConfigCoverage[54] = true
					}
				}
				return 0, nil
			}); err != nil {
				loggerWithConfigCoverage[55] = true
				return
			}
			loggerWithConfigCoverage[56] = true

			if config.Output == nil {
				loggerWithConfigCoverage[57] = true
				_, err = c.Logger().Output().Write(buf.Bytes())
				return
			}
			loggerWithConfigCoverage[58] = true
			_, err = config.Output.Write(buf.Bytes())
			return
		}
	}
}
