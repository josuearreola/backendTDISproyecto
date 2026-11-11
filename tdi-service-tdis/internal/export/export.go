// Package export convierte una db.Tabla a CSV, XLSX o PDF.
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/luuboon/pti-uteq-backend/tdi-service-tdis/internal/db"
	"github.com/xuri/excelize/v2"
)

// Formato describe cómo se sirve cada tipo de archivo.
type Formato struct {
	ContentType string
	Ext         string
	Write       func(io.Writer, *db.Tabla) error
}

var Formatos = map[string]Formato{
	"csv":  {"text/csv; charset=utf-8", "csv", CSV},
	"xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx", XLSX},
	"pdf":  {"application/pdf", "pdf", PDF},
}

// CSV con BOM UTF-8 para que Excel muestre bien los acentos.
func CSV(w io.Writer, t *db.Tabla) error {
	if _, err := w.Write([]byte("\xEF\xBB\xBF")); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(t.Columnas); err != nil {
		return err
	}
	if err := cw.WriteAll(t.Filas); err != nil { // WriteAll ya hace Flush
		return err
	}
	return cw.Error()
}

func XLSX(w io.Writer, t *db.Tabla) error {
	f := excelize.NewFile()
	defer f.Close()

	const hoja = "Reporte"
	f.SetSheetName("Sheet1", hoja)
	sw, err := f.NewStreamWriter(hoja)
	if err != nil {
		return err
	}
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DDEBF7"}}})

	header := make([]any, len(t.Columnas))
	for i, c := range t.Columnas {
		header[i] = excelize.Cell{Value: c, StyleID: bold}
	}
	if err := sw.SetRow("A1", header); err != nil {
		return err
	}
	for i, fila := range t.Filas {
		row := make([]any, len(fila))
		for j, v := range fila {
			row[j] = v
		}
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if err := sw.SetRow(cell, row); err != nil {
			return err
		}
	}
	if err := sw.Flush(); err != nil {
		return err
	}
	if len(t.Columnas) > 0 {
		last, _ := excelize.ColumnNumberToName(len(t.Columnas))
		f.SetColWidth(hoja, "A", last, 20)
	}
	return f.Write(w)
}

// PDF horizontal con columnas de ancho uniforme y textos truncados para que quepan.
// ponytail: ancho uniforme por columna; calcular anchos por contenido si los reportes se ven apretados.
func PDF(w io.Writer, t *db.Tabla) error {
	p := fpdf.New("L", "mm", "Letter", "")
	tr := p.UnicodeTranslatorFromDescriptor("") // las fuentes core son cp1252: traduce acentos y ñ
	p.SetAutoPageBreak(true, 12)
	p.SetFooterFunc(func() {
		p.SetY(-10)
		p.SetFont("Helvetica", "I", 7)
		p.CellFormat(0, 5, tr(fmt.Sprintf("Página %d", p.PageNo())), "", 0, "C", false, 0, "")
	})

	pageW, _ := p.GetPageSize()
	left, _, right, _ := p.GetMargins()
	colW := (pageW - left - right) / float64(max(len(t.Columnas), 1))

	header := func() {
		p.SetFont("Helvetica", "B", 7)
		p.SetFillColor(221, 235, 247)
		for _, c := range t.Columnas {
			p.CellFormat(colW, 6, tr(fit(p, c, colW)), "1", 0, "C", true, 0, "")
		}
		p.Ln(-1)
		p.SetFont("Helvetica", "", 7)
	}

	p.AddPage()
	p.SetFont("Helvetica", "B", 13)
	p.CellFormat(0, 8, tr(t.Titulo), "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "", 8)
	p.CellFormat(0, 5, tr(fmt.Sprintf("Generado: %s · %d registros", time.Now().Format("2006-01-02 15:04"), len(t.Filas))), "", 1, "L", false, 0, "")
	p.Ln(2)
	header()

	_, pageH := p.GetPageSize()
	for _, fila := range t.Filas {
		if p.GetY() > pageH-20 {
			p.AddPage()
			header()
		}
		for _, v := range fila {
			p.CellFormat(colW, 5, tr(fit(p, v, colW)), "1", 0, "L", false, 0, "")
		}
		p.Ln(-1)
	}
	return p.Output(w)
}

// fit recorta el texto con "…" hasta que quepa en el ancho de la celda.
func fit(p *fpdf.Fpdf, s string, w float64) string {
	const pad = 2
	if p.GetStringWidth(s) <= w-pad {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && p.GetStringWidth(string(r)+"...") > w-pad {
		r = r[:len(r)-1]
	}
	return string(r) + "..."
}
