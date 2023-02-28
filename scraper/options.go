package scraper

import "net/http"

type Config struct {
	Mux *http.ServeMux
}

func (c *Config) Handle(pattern string, handler http.Handler) {
	if c.Mux != nil && handler != nil {
		c.Mux.Handle(pattern, handler)
	}
}
