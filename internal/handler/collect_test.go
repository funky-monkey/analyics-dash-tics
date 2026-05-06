package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/service"
)

var _ = Describe("CollectHandler", func() {
	var h *handler.CollectHandler

	BeforeEach(func() {
		geo := service.NewGeoLocator("")
		fp := service.NewFingerprinter("test-salt-32-bytes-xxxxxxxxxxxxxxxx")
		collectSvc := service.NewCollectService(geo, fp)
		h = handler.NewCollectHandler(collectSvc, nil)
	})

	Describe("POST /collect", func() {
		Context("with valid JSON payload", func() {
			It("never returns 5xx synchronously", func() {
				payload := map[string]any{
					"site": "tk_nonexistent", "type": "pageview",
					"url": "https://example.com/", "referrer": "",
				}
				body, _ := json.Marshal(payload)
				req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.RemoteAddr = "1.2.3.4:1234"
				rec := httptest.NewRecorder()

				h.Collect(rec, req)

				Expect(rec.Code).To(BeNumerically(">=", 200))
				Expect(rec.Code).To(BeNumerically("<", 500))
			})
		})

		Context("with missing site token", func() {
			It("returns 400 Bad Request", func() {
				payload := map[string]any{"type": "pageview", "url": "https://example.com/"}
				body, _ := json.Marshal(payload)
				req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				h.Collect(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest))
			})
		})

		Context("with bot User-Agent", func() {
			It("returns 202 without writing", func() {
				payload := map[string]any{
					"site": "tk_any", "type": "pageview", "url": "https://example.com/",
				}
				body, _ := json.Marshal(payload)
				req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "Googlebot/2.1")
				rec := httptest.NewRecorder()

				h.Collect(rec, req)

				Expect(rec.Code).To(Equal(http.StatusAccepted))
			})
		})
	})
})
