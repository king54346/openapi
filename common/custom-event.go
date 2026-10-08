package common

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Server-Sent Events，按原样写出 Data 字段
// http://www.w3.org/TR/2009/WD-eventsource-20091029/

var contentType = []string{"text/event-stream"}
var noCache = []string{"no-cache"}

var dataReplacer = strings.NewReplacer(
	"\n", "\n",
	"\r", "\r")

type CustomEvent struct {
	Data any
}

func writeData(w io.Writer, data any) error {
	s := fmt.Sprint(data)
	if _, err := dataReplacer.WriteString(w, s); err != nil {
		return err
	}
	if strings.HasPrefix(s, "data") {
		if _, err := io.WriteString(w, "\n\n"); err != nil {
			return err
		}
	}
	return nil
}

func (r CustomEvent) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	return writeData(w, r.Data)
}

func (r CustomEvent) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	header["Content-Type"] = contentType
	if _, exist := header["Cache-Control"]; !exist {
		header["Cache-Control"] = noCache
	}
}
