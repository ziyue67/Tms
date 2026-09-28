package httpapi

import (
	"encoding/json"
	"net/http"
	"time"
)

type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	TS   int64  `json:"ts"`
	Data any    `json:"data"`
}

func OK(data any) Response {
	return Response{Code: 0, Msg: "操作成功", TS: time.Now().UnixMilli(), Data: data}
}

func Failure(message string) Response {
	return Error(-1, message)
}

func Error(code int, message string) Response {
	return Response{Code: code, Msg: message, TS: time.Now().UnixMilli(), Data: nil}
}

func writeResponse(w http.ResponseWriter, response Response) {
	writeResponseStatus(w, http.StatusOK, response)
}

func writeResponseStatus(w http.ResponseWriter, status int, response Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(destination); err != nil {
		writeResponse(w, Error(500, "请求参数格式错误"))
		return false
	}
	return true
}
