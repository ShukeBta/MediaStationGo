package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type embyAuthByNameReq struct {
	Username     string `json:"Username"`
	Pw           string `json:"Pw"`
	Password     string `json:"Password"`
	PasswordMd5  string `json:"PasswordMd5"`
	PasswordSha1 string `json:"PasswordSha1"`
}

func parseEmbyAuthByNameReq(c *gin.Context) (embyAuthByNameReq, error) {
	req := embyAuthByNameReq{}
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "json") {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
			return req, err
		}
		fillEmbyAuthFromMap(&req, body)
	}

	if req.Username == "" || (req.Pw == "" && req.Password == "" && req.PasswordMd5 == "" && req.PasswordSha1 == "") {
		_ = c.Request.ParseForm()
		if req.Username == "" {
			req.Username = firstFormValue(c, "Username", "username", "Name", "name")
		}
		if req.Pw == "" {
			req.Pw = firstFormValue(c, "Pw", "pw")
		}
		if req.Password == "" {
			req.Password = firstFormValue(c, "Password", "password")
		}
		if req.PasswordMd5 == "" {
			req.PasswordMd5 = firstFormValue(c, "PasswordMd5", "passwordMd5", "password_md5")
		}
		if req.PasswordSha1 == "" {
			req.PasswordSha1 = firstFormValue(c, "PasswordSha1", "passwordSha1", "password_sha1")
		}
	}

	if req.Username == "" {
		req.Username = firstQueryValue(c, "Username", "username", "Name", "name")
	}
	if req.Pw == "" {
		req.Pw = firstQueryValue(c, "Pw", "pw")
	}
	if req.Password == "" {
		req.Password = firstQueryValue(c, "Password", "password")
	}
	if req.PasswordMd5 == "" {
		req.PasswordMd5 = firstQueryValue(c, "PasswordMd5", "passwordMd5", "password_md5")
	}
	if req.PasswordSha1 == "" {
		req.PasswordSha1 = firstQueryValue(c, "PasswordSha1", "passwordSha1", "password_sha1")
	}
	if req.Username == "" || (req.Pw == "" && req.Password == "" && req.PasswordMd5 == "" && req.PasswordSha1 == "") {
		fillEmbyAuthFromRawBody(c, &req)
	}
	return req, nil
}

func fillEmbyAuthFromMap(req *embyAuthByNameReq, body map[string]any) {
	if req.Username == "" {
		req.Username = firstStringFromMap(body, "Username", "username", "UserName", "userName", "Name", "name", "LoginName", "loginName")
	}
	if req.Pw == "" {
		req.Pw = firstStringFromMap(body, "Pw", "pw", "PW")
	}
	if req.Password == "" {
		req.Password = firstStringFromMap(body, "Password", "password", "Pass", "pass", "Pwd", "pwd")
	}
	if req.PasswordMd5 == "" {
		req.PasswordMd5 = firstStringFromMap(body, "PasswordMd5", "passwordMd5", "password_md5")
	}
	if req.PasswordSha1 == "" {
		req.PasswordSha1 = firstStringFromMap(body, "PasswordSha1", "passwordSha1", "password_sha1")
	}
}

func fillEmbyAuthFromRawBody(c *gin.Context, req *embyAuthByNameReq) {
	if c.Request == nil || c.Request.Body == nil {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return
	}
	if bytes.HasPrefix(raw, []byte("{")) {
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err == nil {
			fillEmbyAuthFromMap(req, body)
		}
		return
	}
	if values, err := url.ParseQuery(string(raw)); err == nil {
		fillEmbyAuthFromValues(req, values)
	}
}

func fillEmbyAuthFromValues(req *embyAuthByNameReq, values url.Values) {
	if req.Username == "" {
		req.Username = firstValue(values, "Username", "username", "UserName", "userName", "Name", "name", "LoginName", "loginName")
	}
	if req.Pw == "" {
		req.Pw = firstValue(values, "Pw", "pw", "PW")
	}
	if req.Password == "" {
		req.Password = firstValue(values, "Password", "password", "Pass", "pass", "Pwd", "pwd")
	}
	if req.PasswordMd5 == "" {
		req.PasswordMd5 = firstValue(values, "PasswordMd5", "passwordMd5", "password_md5")
	}
	if req.PasswordSha1 == "" {
		req.PasswordSha1 = firstValue(values, "PasswordSha1", "passwordSha1", "password_sha1")
	}
}

func firstValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func firstStringFromMap(body map[string]any, keys ...string) string {
	if len(body) == 0 {
		return ""
	}
	for _, key := range keys {
		if value, ok := body[key]; ok {
			if s, ok := value.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstFormValue(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if values, ok := c.Request.PostForm[key]; ok && len(values) > 0 {
			if value := strings.TrimSpace(values[0]); value != "" {
				return value
			}
		}
	}
	return ""
}

func firstQueryValue(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
	}
	return ""
}
