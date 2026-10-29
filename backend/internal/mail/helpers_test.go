package mail_test

import (
	"encoding/base64"
	"io"
	"strings"
)

func base64Reader(s string) io.Reader {
	return base64.NewDecoder(base64.StdEncoding, strings.NewReader(strings.ReplaceAll(s, "\r\n", "")))
}
