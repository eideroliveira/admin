package pagebuilder

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/qor5/web/v3"
	"github.com/sunfmin/reflectutils"
	"gorm.io/gorm"
)

type PageExport struct {
	Page       Page              `json:"page"`
	Containers []ContainerExport `json:"containers"`
}

type ContainerExport struct {
	Container Container              `json:"container"`
	Content   map[string]interface{} `json:"content"`
}

func (b *Builder) exportPages(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		http.Error(w, "No IDs provided", http.StatusBadRequest)
		return
	}
	ids := strings.Split(idsParam, ",")
	var pages []Page
	for _, id := range ids {
		segs := strings.Split(id, "_")
		if len(segs) < 2 {
			continue
		}
		pageID := segs[0]
		version := segs[1]
		tx := b.db.Where("id = ? AND version = ?", pageID, version)
		if len(segs) > 2 {
			tx = tx.Where("locale_code = ?", segs[2])
		}
		var p Page
		if err := tx.First(&p).Error; err == nil {
			pages = append(pages, p)
		}
	}

	var exportData = make([]PageExport, 0)
	for _, p := range pages {
		var containers []Container
		b.db.Where("page_id = ? AND page_version = ?", p.ID, p.Version.Version).Order("display_order ASC").Find(&containers)

		var containerExports []ContainerExport
		for _, c := range containers {
			cb := b.ContainerByName(c.ModelName)
			if cb == nil {
				continue
			}
			model := cb.mb.NewModel()
			if err := b.db.First(model, c.ModelID).Error; err != nil {
				continue
			}

			// Marshal model to map
			jsonBytes, _ := json.Marshal(model)
			var content map[string]interface{}
			json.Unmarshal(jsonBytes, &content)

			containerExports = append(containerExports, ContainerExport{
				Container: c,
				Content:   content,
			})
		}
		exportData = append(exportData, PageExport{
			Page:       p,
			Containers: containerExports,
		})
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"pages_export_%s.json\"", time.Now().Format("20060102150405")))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exportData)
}

func (b *Builder) exportPagesEvent(ctx *web.EventContext) (r web.EventResponse, err error) {
	ids := ctx.R.FormValue("ids")
	if ids == "" {
		return
	}
	url := fmt.Sprintf("%s/export/pages?ids=%s", b.prefix, ids)
	web.AppendRunScripts(&r, fmt.Sprintf("window.open('%s', '_blank')", url))
	return
}

func (b *Builder) importPages(ctx *web.EventContext) (r web.EventResponse, err error) {
	var (
		db = b.db
	)

	if len(ctx.R.MultipartForm.File["ImportFile"]) == 0 {
		web.AppendRunScripts(&r, `locals.importDialog = false; form.ImportFile = null;`)
		return
	}

	file, _, err := ctx.R.FormFile("ImportFile")
	if err != nil {
		return
	}
	defer file.Close()

	var importData []PageExport
	if err = json.NewDecoder(file).Decode(&importData); err != nil {
		return
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, pData := range importData {
			page := pData.Page
			page.ID = 0
			page.Slug = fmt.Sprintf("%s-imported-%d", page.Slug, time.Now().UnixNano())
			page.Version.Version = time.Now().Format("2006-01-02-15-04-05")
			page.Version.VersionName = "Imported"
			
			if err := tx.Create(&page).Error; err != nil {
				return err
			}

			for _, cData := range pData.Containers {
				container := cData.Container
				container.ID = 0
				container.PageID = page.ID
				container.PageVersion = page.Version.Version

				cb := b.ContainerByName(container.ModelName)
				if cb == nil {
					continue
				}
				model := cb.mb.NewModel()
				
				// Unmarshal content to model
				contentBytes, _ := json.Marshal(cData.Content)
				json.Unmarshal(contentBytes, model)

				// Reset ID of content model
				reflectutils.Set(model, "ID", uint(0))

				if err := tx.Create(model).Error; err != nil {
					return err
				}

				container.ModelID = reflectutils.MustGet(model, "ID").(uint)
				if err := tx.Create(&container).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return
	}

	r.Reload = true
	return
}
