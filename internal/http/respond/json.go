package respond

import (
	"net/http"

	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/response"
)

func JSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	if err := response.JSON(w, status, value); err != nil {
		domainerror.Write(w, r, err)
	}
}
