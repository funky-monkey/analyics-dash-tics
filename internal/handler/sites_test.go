package handler_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/service"
)

var _ = Describe("SitesHandler", func() {
	var h *handler.SitesHandler

	BeforeEach(func() {
		authSvc := service.NewAuth(
			[]byte("test-access-secret-32-bytes-xxxxx"),
			[]byte("test-refresh-secret-32-bytes-xxxx"),
		)
		h = handler.NewSitesHandler(authSvc, nil, "https://dash.local")
	})

	Describe("GET /account/sites/new", func() {
		It("returns 200 OK", func() {
			req := httptest.NewRequest(http.MethodGet, "/account/sites/new", nil)
			rec := httptest.NewRecorder()

			h.NewSitePage(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("POST /account/sites/new", func() {
		Context("with empty name", func() {
			It("returns 422", func() {
				form := url.Values{
					"name":   {""},
					"domain": {"example.com"},
					"_csrf":  {"token"},
				}
				req := httptest.NewRequest(http.MethodPost, "/account/sites/new", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
				rec := httptest.NewRecorder()

				h.CreateSite(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})

		Context("with empty domain", func() {
			It("returns 422", func() {
				form := url.Values{
					"name":   {"My Site"},
					"domain": {""},
					"_csrf":  {"token"},
				}
				req := httptest.NewRequest(http.MethodPost, "/account/sites/new", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
				rec := httptest.NewRecorder()

				h.CreateSite(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})
	})
})
