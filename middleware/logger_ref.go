func LoggerWithConfig(config LoggerConfig) echo.MiddlewareFunc {
	if config.Skipper == nil{
		config.Skipper = DefaultLoggerConfig.Skipper
	}
	if config.Format == "" {
		config.Format = DefaultLoggerConfig.Format
	}
	if config.Output == nil {
		config.Output = DefaultLoggerConfig.Output
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
		return func(c echo.Context) error {
			// Se Skipper è vero, salta il middleware
			if config.Skipper(c) {
				return next(c)
			}
			// Processa la richiesta
			return processRequest(c, next, &config)
		}
	}
}

func processRequest(c echo.Context, next echo.HandlerFunc, config *LoggerConfig) error {
	req := c.Request()
	res := c.Response()
	start := time.Now()
	err := next(c)
	if err != nil {
		c.Error(err)
	}
	stop := time.Now()

	// Gestione buffer separata
	buf := config.pool.Get().(*bytes.Buffer)
	buf.Reset()
	defer config.pool.Put(buf)

	// Processa il template
	if _, err = processTemplate(buf, c, req, res, config, start, stop); err != nil {
		return err
	}

	// Scrive l'output
	return writeOutput(buf, c, config)
}


func processTemplate(buf *bytes.Buffer, c echo.Context, req *http.Request, res *echo.Response, config *LoggerConfig, start, stop time.Time) (int, error) {
	return config.template.ExecuteFunc(buf, func(w io.Writer, tag string) (int, error) {
		return handleTag(buf, tag, c, req, res, config, start, stop)
	})
}

func writeOutput(buf *bytes.Buffer, c echo.Context, config *LoggerConfig) error {
	if config.Output == nil {
		_, err := c.Logger().Output().Write(buf.Bytes())
		return err
	}
	_, err := config.Output.Write(buf.Bytes())
	return err
}

func handleTag(buf *bytes.Buffer, tag string, c echo.Context, req *http.Request, res *echo.Response, config *LoggerConfig, start, stop time.Time) (int, error) {
	// Gestione del tag "custom"
	if tag == "custom" {
		if config.CustomTagFunc != nil {
			return config.CustomTagFunc(c, buf)
		}
		return 0, nil
	}

	// Gestione dei tag "time_unix", "latency", "latency_human", "status"
	if tag == "time_unix" {
		return buf.WriteString(strconv.FormatInt(time.Now().Unix(), 10))
	} else if tag == "latency" {
		return buf.WriteString(strconv.FormatInt(int64(stop.Sub(start)), 10))
	} else if tag == "latency_human" {
		return buf.WriteString(stop.Sub(start).String())
	} else if tag == "status" {
		return buf.WriteString(getColoredStatus(res.Status, config))
	}

	// Gestione dei tag dinamici
	if strings.HasPrefix(tag, "header:") {
		return buf.Write([]byte(req.Header.Get(tag[7:])))
	} else if strings.HasPrefix(tag, "query:") {
		return buf.Write([]byte(c.QueryParam(tag[6:])))
	} else if strings.HasPrefix(tag, "form:") {
		return buf.Write([]byte(c.FormValue(tag[5:])))
	} else if strings.HasPrefix(tag, "cookie:") {
		cookie, err := c.Cookie(tag[7:])
		if err == nil {
			return buf.Write([]byte(cookie.Value))
		}
	}

	// Se nessun caso è stato gestito
	return 0, nil
}


func getColoredStatus(status int, config *LoggerConfig) string {
	s := config.colorer.Green(status)
	switch {
	case status >= 500:
		s = config.colorer.Red(status)
	case status >= 400:
		s = config.colorer.Yellow(status)
	case status >= 300:
		s = config.colorer.Cyan(status)
	}
	return s
}
