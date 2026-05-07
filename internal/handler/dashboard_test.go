package handler_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/service"
)

var _ = Describe("DashboardHandler", func() {
	var h *handler.DashboardHandler

	BeforeEach(func() {
		authSvc := service.NewAuth(
			[]byte("test-access-secret-32-bytes-xxxxx"),
			[]byte("test-refresh-secret-32-bytes-xxxx"),
		)
		h = handler.NewDashboardHandler(authSvc, nil, "")
	})

	Describe("GET /dashboard", func() {
		Context("when repos is nil (no sites)", func() {
			It("redirects to /account/sites/new", func() {
				req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
				ctx := middleware.WithUserID(req.Context(), "user-123")
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()

				h.Aggregate(rec, req)

				Expect(rec.Code).To(Equal(http.StatusSeeOther))
				Expect(rec.Header().Get("Location")).To(Equal("/account/sites/new"))
			})
		})
	})

	Describe("GET /sites/:siteID/overview", func() {
		Context("when repos is nil", func() {
			It("returns 404", func() {
				req := httptest.NewRequest(http.MethodGet, "/sites/nonexistent/overview", nil)
				ctx := middleware.WithUserID(req.Context(), "user-123")
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()

				h.Overview(rec, req)

				Expect(rec.Code).To(Equal(http.StatusNotFound))
			})
		})
	})
})
