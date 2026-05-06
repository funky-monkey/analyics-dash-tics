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

func newTestAuthHandler() *handler.AuthHandler {
	authSvc := service.NewAuth(
		[]byte("test-access-secret-32-bytes-xxxxx"),
		[]byte("test-refresh-secret-32-bytes-xxxx"),
	)
	// nil repos — tests only cover paths that don't hit the database
	return handler.NewAuthHandler(authSvc, nil, "https://dash.local")
}

var _ = Describe("AuthHandler", func() {

	Describe("GET /login", func() {
		It("returns 200 and text/html", func() {
			h := newTestAuthHandler()
			req := httptest.NewRequest(http.MethodGet, "/login", nil)
			rec := httptest.NewRecorder()

			h.LoginPage(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("text/html"))
		})
	})

	Describe("POST /signup", func() {
		Context("with a password shorter than 12 characters", func() {
			It("returns 422 Unprocessable Entity", func() {
				h := newTestAuthHandler()
				form := url.Values{
					"name":     {"Test User"},
					"email":    {"test@example.com"},
					"password": {"short"},
					"_csrf":    {"token"},
				}
				req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
				rec := httptest.NewRecorder()

				h.Signup(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})

		Context("with missing name", func() {
			It("returns 422 Unprocessable Entity", func() {
				h := newTestAuthHandler()
				form := url.Values{
					"name":     {""},
					"email":    {"test@example.com"},
					"password": {"strongpassword123"},
					"_csrf":    {"token"},
				}
				req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
				rec := httptest.NewRecorder()

				h.Signup(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})
	})

	Describe("POST /logout", func() {
		It("clears auth cookies and redirects to /login", func() {
			h := newTestAuthHandler()
			req := httptest.NewRequest(http.MethodPost, "/logout", nil)
			req.AddCookie(&http.Cookie{Name: "access_token", Value: "sometoken"})
			req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
			rec := httptest.NewRecorder()

			h.Logout(rec, req)

			Expect(rec.Code).To(Equal(http.StatusSeeOther))
			Expect(rec.Header().Get("Location")).To(Equal("/login"))

			// Verify cookies are cleared (MaxAge = -1 means delete)
			cookies := rec.Result().Cookies()
			var clearedNames []string
			for _, c := range cookies {
				if c.MaxAge == -1 {
					clearedNames = append(clearedNames, c.Name)
				}
			}
			Expect(clearedNames).To(ContainElements("access_token", "refresh_token"))
		})
	})
})
