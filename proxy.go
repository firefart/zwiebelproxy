package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (app *application) modifyResponse(resp *http.Response) error {
	app.logger.Debug("entered modifyResponse for %s with status %d", resp.Request.URL.String(), resp.StatusCode)

	app.logger.Debugf("Header: %#v", resp.Header)
	for k, v := range resp.Header {
		k = strings.ReplaceAll(k, ".onion", app.domain)
		resp.Header[k] = []string{}
		for _, v2 := range v {
			v2 = strings.ReplaceAll(v2, ".onion", app.domain)
			resp.Header[k] = append(resp.Header[k], v2)
		}
	}

	// no body modification on file downloads
	contentDisp, ok := resp.Header["Content-Disposition"]
	if ok && len(contentDisp) > 0 && strings.HasPrefix(contentDisp[0], "attachment") {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error on reading body: %w", err)
	}
	app.logger.Debugf("%s: Got a %d body len", resp.Request.URL.String(), len(body))
	// replace stuff for domain replacement
	body = bytes.ReplaceAll(body, []byte(`.onion/`), []byte(fmt.Sprintf(`%s/`, app.domain)))
	body = bytes.ReplaceAll(body, []byte(`.onion"`), []byte(fmt.Sprintf(`%s"`, app.domain)))

	// body can be read only once so recreate a new reader
	resp.Body = io.NopCloser(bytes.NewBuffer(body))
	// update the content-length to our new body
	resp.Header["Content-Length"] = []string{fmt.Sprint(len(body))}
	return nil
}
