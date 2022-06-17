package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (app *application) modifyResponse(resp *http.Response) error {
	app.logger.Debug("entered modifyResponse")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error on reading body: %w", err)
	}

	app.logger.Debugf("%s: Got a %d body len with a status of %d", resp.Request.URL.String(), len(body), resp.StatusCode)

	// replace stuff for domain replacement
	body = bytes.ReplaceAll(body, []byte(`.onion/`), []byte(fmt.Sprintf(`%s/`, app.domain)))
	body = bytes.ReplaceAll(body, []byte(`.onion"`), []byte(fmt.Sprintf(`%s"`, app.domain)))

	app.logger.Debugf("Header: %#v", resp.Header)

	for k, v := range resp.Header {
		k = strings.ReplaceAll(k, ".onion", app.domain)
		resp.Header[k] = []string{}
		for _, v2 := range v {
			v2 = strings.ReplaceAll(v2, ".onion", app.domain)
			resp.Header[k] = append(resp.Header[k], v2)
		}
	}

	app.logger.Debugf("modified body")

	resp.Body = io.NopCloser(bytes.NewBuffer(body))
	resp.Header["Content-Length"] = []string{fmt.Sprint(len(body))}
	return nil
}
