package utils

import (
	"fmt"
	"reflect"
	"strings"
)

const maxLineLength = 120

type ASCIITableHeader struct {
	Name      string // name in table header
	Field     string // attribute name in data row
	Alignment string // flag whether column is aligned to the right
	Size      int    // calculated max Size of column
}

// ASCIITable creates an ascii table from columns and data rows
func ASCIITable(header []ASCIITableHeader, rows any, escapePipes bool) (string, error) {
	dataRows := reflect.ValueOf(rows)
	if dataRows.Kind() != reflect.Slice {
		return "", fmt.Errorf("rows is not a slice")
	}

	err := calculateHeaderSize(header, dataRows, escapePipes)
	if err != nil {
		return "", err
	}

	// output header
	out := ""
	strBuilder := strings.Builder{}
	for _, head := range header {
		strBuilder.WriteString(fmt.Sprintf(fmt.Sprintf("| %%-%ds ", head.Size), head.Name))
	}
	out += strBuilder.String() + "|\n"

	// output separator
	strBuilder.Reset()
	for _, head := range header {
		padding := " "
		strBuilder.WriteString(fmt.Sprintf("|%s%s%s", padding, strings.Repeat("-", head.Size), padding))
	}
	out += strBuilder.String() + "|\n"
	strBuilder.Reset()

	// output data
	for i := range dataRows.Len() {
		rowVal := dataRows.Index(i)
		for _, head := range header {
			value, _ := asciiTableRowValue(escapePipes, rowVal, head)

			switch head.Alignment {
			case "right":
				strBuilder.WriteString(fmt.Sprintf(fmt.Sprintf("| %%%ds ", head.Size), value))
			case "left", "":
				strBuilder.WriteString(fmt.Sprintf(fmt.Sprintf("| %%-%ds ", head.Size), value))
			case "centered":
				padding := (head.Size - len(value)) / 2
				strBuilder.WriteString(fmt.Sprintf("| %*s%-*s ", padding, "", head.Size-padding, value))
			default:
				err := fmt.Errorf("unsupported alignment '%s' in table", head.Alignment)

				return "", err
			}
		}
		strBuilder.WriteString("|\n")
	}

	out += strBuilder.String()
	strBuilder.Reset()

	return out, nil
}

func asciiTableRowValue(escape bool, rowVal reflect.Value, head ASCIITableHeader) (string, error) {
	value := ""
	field := rowVal.FieldByName(head.Field)
	if field.IsValid() {
		t := field.Type().String()
		switch t {
		case "string":
			value = field.String()
		default:
			return "", fmt.Errorf("unsupported struct attribute type for field %s: %s", head.Field, t)
		}
	}

	if escape {
		value = strings.ReplaceAll(value, "\n", `\n`)
		value = strings.ReplaceAll(value, "|", "\\|")
		value = strings.ReplaceAll(value, "$", "\\$")
		value = strings.ReplaceAll(value, "*", "\\*")
	}

	return value, nil
}

func calculateHeaderSize(header []ASCIITableHeader, dataRows reflect.Value, escapePipes bool) error {
	// set headers as minimum Size
	for i, head := range header {
		header[i].Size = len(head.Name)
	}

	// adjust column Size from max row data
	for i := range dataRows.Len() {
		rowVal := dataRows.Index(i)
		if rowVal.Kind() != reflect.Struct {
			return fmt.Errorf("row %d is not a struct", i)
		}
		for num, head := range header {
			value, err := asciiTableRowValue(escapePipes, rowVal, head)
			if err != nil {
				return err
			}
			length := len(value)
			if length > header[num].Size {
				header[num].Size = length
			}
		}
	}

	// calculate total line length
	total := 0
	for i := range header {
		total += header[i].Size + 3 // add padding
	}

	if total < maxLineLength {
		return nil
	}

	avgAvail := maxLineLength / len(header)
	tooWide := []int{}
	sumTooWide := 0
	for i := range header {
		if header[i].Size > avgAvail {
			tooWide = append(tooWide, i)
			sumTooWide += header[i].Size
		}
	}
	avgLargeCol := (maxLineLength - (total - sumTooWide)) / len(tooWide)
	for _, i := range tooWide {
		header[i].Size = avgLargeCol
	}

	return nil
}
