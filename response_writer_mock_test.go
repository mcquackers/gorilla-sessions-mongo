package sessions_mongo

import (
	"bytes"
	"net/http"
)

type RWAssertableInput struct {
	StatusCode int
	Buffer     *bytes.Buffer
	Headers    http.Header
}

type RWMockableOutput struct {
}

type ResponseWriterMock struct {
	Input  *RWAssertableInput
	Output *RWMockableOutput
}

func NewMockResponseWriter() *ResponseWriterMock {
	return &ResponseWriterMock{
		Input: &RWAssertableInput{
			Buffer:  bytes.NewBuffer(nil),
			Headers: http.Header(make(map[string][]string)),
		},
		Output: &RWMockableOutput{},
	}
}

func (w *ResponseWriterMock) Write(bytesToWrite []byte) (int, error) {
	return w.Input.Buffer.Write(bytesToWrite)
}

func (w *ResponseWriterMock) Header() http.Header {
	return w.Input.Headers
}

func (w *ResponseWriterMock) WriteHeader(statusCode int) {
	w.Input.StatusCode = statusCode
}

func (w *ResponseWriterMock) GetWrittenBytes() []byte {
	return w.Input.Buffer.Bytes()
}

func (w *ResponseWriterMock) GetStatusCode() int {
	return w.Input.StatusCode
}

func (w *ResponseWriterMock) GetHeaders() http.Header {
	return w.Input.Headers
}

func (w *ResponseWriterMock) Reset() {
	w.Input.Buffer.Reset()
	w.Input.StatusCode = 0
	w.Input.Headers = http.Header{}
}
