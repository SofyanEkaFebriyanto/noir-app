// F-13: endpoint "Tentang Noir" — lihat & edit persona.
// GET    /v1/persona        → persona efektif saat ini
// PUT    /v1/persona        → set override {prompt}
// DELETE /v1/persona        → reset ke default
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/persona"
)

func (s *Server) registerPersona(r *gin.Engine) {
	r.GET("/v1/persona", func(c *gin.Context) {
		overridden := false
		prompt := s.deps.SystemPrompt
		if s.deps.Config.SystemPrompt != "" {
			prompt = s.deps.Config.SystemPrompt
		}
		if v, err := s.deps.Store.GetSetting("persona_override"); err == nil && v != "" {
			prompt = v
			overridden = true
		}
		c.JSON(http.StatusOK, gin.H{
			"prompt":     prompt,
			"overridden": overridden,
			"default":    persona.SystemPrompt(),
		})
	})
	r.PUT("/v1/persona", func(c *gin.Context) {
		var in struct {
			Prompt string `json:"prompt"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || len(in.Prompt) < 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt minimal 10 karakter"})
			return
		}
		if len(in.Prompt) > 8000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt maksimal 8000 karakter"})
			return
		}
		if err := s.deps.Store.SetSetting("persona_override", in.Prompt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "overridden": true})
	})
	r.DELETE("/v1/persona", func(c *gin.Context) {
		if err := s.deps.Store.DelSetting("persona_override"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mereset"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "overridden": false})
	})
}
