package export

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/luuboon/pti-uteq-backend/tdi-service-tdis/internal/db"
	"github.com/xuri/excelize/v2"
)

var tabla = &db.Tabla{
	Titulo:   "Registros de TDI",
	Columnas: []string{"Matrícula", "Alumno", "Comentario"},
	Filas: [][]string{
		{"202337153", "José Peña Núñez", "con, coma y \"comillas\""},
		{"2020371072", "Ana", strings.Repeat("texto largo ", 30)},
	},
}

func TestFormatos(t *testing.T) {
	for nombre, f := range Formatos {
		var buf bytes.Buffer
		if err := f.Write(&buf, tabla); err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		if buf.Len() == 0 {
			t.Fatalf("%s: salida vacía", nombre)
		}
		switch nombre {
		case "csv":
			rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(buf.Bytes(), []byte("\xEF\xBB\xBF")))).ReadAll()
			if err != nil || len(rows) != 3 || rows[1][2] != tabla.Filas[0][2] || rows[1][1] != "José Peña Núñez" {
				t.Fatalf("csv no hace round-trip: %v %v", rows, err)
			}
		case "xlsx":
			x, err := excelize.OpenReader(&buf)
			if err != nil {
				t.Fatal(err)
			}
			if v, _ := x.GetCellValue("Reporte", "B2"); v != "José Peña Núñez" {
				t.Fatalf("xlsx B2 = %q", v)
			}
		case "pdf":
			if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
				t.Fatal("pdf sin cabecera %PDF-")
			}
		}
	}
}
