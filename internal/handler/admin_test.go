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

var _ = Describe("AdminHandler", func() {
	var h *handler.AdminHandler

	BeforeEach(func() {
		authSvc := service.NewAuth(
			[]byte("test-access-secret-32-bytes-xxxxx"),
			[]byte("test-refresh-secret-32-bytes-xxxx"),
		)
		h = handler.NewAdminHandler(authSvc, nil)
	})

	Describe("GET /admin", func() {
		It("returns 200", func() {
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			ctx := middleware.WithUserID(req.Context(), "admin-123")
			ctx = middleware.WithRole(ctx, "admin")
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			h.Index(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("GET /admin/users", func() {
		It("returns 200", func() {
			req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
			ctx := middleware.WithUserID(req.Context(), "admin-123")
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			h.Users(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})
})
