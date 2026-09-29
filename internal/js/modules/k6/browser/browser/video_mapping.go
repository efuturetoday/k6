package browser

import (
	"github.com/grafana/sobek"

	"go.k6.io/k6/internal/js/modules/k6/browser/common"
)

func mapVideo(vu moduleVU, v *common.Video) mapping {
	if v == nil {
		return nil
	}
	return mapping{
		"path": func() *sobek.Promise {
			return promise(vu, func() (any, error) {
				return v.Path(), nil
			})
		},
		"saveAs": func(path string) *sobek.Promise {
			return promise(vu, func() (any, error) {
				return nil, v.SaveAs(path)
			})
		},
		"delete": func() *sobek.Promise {
			return promise(vu, func() (any, error) {
				return nil, v.Delete()
			})
		},
	}
}
