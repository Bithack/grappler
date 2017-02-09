package main

import (
	"fmt"
	"strings"
	"teorem/multimatrix/matchar"

	"github.com/gonum/matrix/mat64"
)

type variable struct {
	T           string
	FloatMatrix *mat64.Dense
	CharMatrix  *matchar.Matchar
	Message     *caffeMessage
}

func newVariableFromMessage(m *caffeMessage) (v *variable) {
	v = new(variable)
	v.T = "Message"
	v.Message = m
	return
}

func newVariableFromFloat(m *mat64.Dense) (v *variable) {
	v = new(variable)
	v.T = "FloatMatrix"
	v.FloatMatrix = m
	return
}

func newVariableFromChar(m *matchar.Matchar) (v *variable) {
	v = new(variable)
	v.T = "CharMatrix"
	v.CharMatrix = m
	return
}

func (v *variable) IsChar() (r bool) {
	if v.T == "CharMatrix" {
		r = true
	}
	return
}

func (v *variable) IsFloat() (r bool) {
	if v.T == "FloatMatrix" {
		r = true
	}
	return
}

func (v *variable) CheckDims(r, c int) (ret bool) {
	switch v.T {
	case "FloatMatrix":
		r2, c2 := v.FloatMatrix.Dims()
		if (r2 == r) || (c2 == c) {
			ret = true
		}
	}
	return
}

func (v *variable) GetScalar() (s float64) {
	switch {
	case v.IsScalar():
		s = v.FloatMatrix.At(0, 0)
	}
	return
}
func (v *variable) IsScalar() (r bool) {
	switch {
	case v.T == "FloatMatrix" && v.CheckDims(1, 1):
		r = true
	}
	return
}

func (v *variable) IsMessage() (r bool) {
	if v.T == "Message" {
		r = true
	}
	return
}

func (v *variable) Type() (s string) {
	switch v.T {
	case "FloatMatrix":
		r, c := v.FloatMatrix.Dims()
		s = fmt.Sprintf("Float64 (%v, %v)", r, c)
	case "CharMatrix":
		r, c := v.CharMatrix.Dims()
		s = fmt.Sprintf("Char (%v, %v)", r, c)
	case "Message":
		if v.Message != nil {
			s = v.T + "." + v.Message.T
		} else {
			s = v.T
		}
	}
	return
}

func (v *variable) Clone() (v2 *variable) {
	switch v.T {
	case "Message":
		v2 = newVariableFromMessage(v.Message.Clone())
	case "FloatMatrix":
		//v2 = newVariableFromFloat(v.FloatMatrix.Clone())
	case "CharMatrix":
		//v2 = newVariableFromChar(v.CharMatrix)
	}
	return
}

func (v *variable) GetField(f string) (v2 *variable) {
	grLogs("GetField %v.%v", v.T, f)
	switch v.T {
	case "Message":
		v2 = v.Message.GetField(f)
	}
	return
}

func (v *variable) GetFields() (s []string) {
	switch v.T {
	case "Message":
		s = v.Message.GetFields()
	}
	return
}

func (v *variable) Print(name string) {
	switch v.T {
	case "Message":
		v.Message.Print()

	case "FloatMatrix":
		r, c := v.FloatMatrix.Dims()
		switch {
		case r == 0 && c == 0:
			fmt.Printf("%s = []\n", name)
		case r == 1 && c == 1:
			fmt.Printf("%s = %.4f\n", name, v.FloatMatrix.At(0, 0))
		default:
			s := int(maxPrintWidth / 2)
			fa := mat64.Formatted(v.FloatMatrix, mat64.Excerpt(s))
			///mat64.Format(matrixes[mat], )
			fmt.Printf("%s =\n%.4f\n", name, fa)
		}

	case "CharMatrix":
		r, c := v.CharMatrix.Dims()
		switch {
		case r == 0 && c == 0:
			fmt.Printf("%s = []\n", name)
		default:
			fmt.Printf("%s =\n", name)
			if r > 10 {
				fmt.Printf("Dims(%v, 1)\n", r)
				str := v.CharMatrix.RowView(0)
				fmt.Printf("⎡%s"+strings.Repeat(" ", c-len(str))+"⎤\n", str)
				for i := 1; i < 5; i++ {
					str = v.CharMatrix.RowView(i)
					fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
				}
				fmt.Printf(" .\n .\n .\n")
				for i := r - 5; i < r-1; i++ {
					str = v.CharMatrix.RowView(i)
					fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
				}
				str = v.CharMatrix.RowView(r - 1)
				fmt.Printf("⎣%s"+strings.Repeat(" ", c-len(str))+"⎦\n", str)

			} else {
				str := v.CharMatrix.RowView(0)
				fmt.Printf("⎡%s"+strings.Repeat(" ", c-len(str))+"⎤\n", str)
				for i := 1; i < r-1; i++ {
					str = v.CharMatrix.RowView(i)
					fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
				}
				str = v.CharMatrix.RowView(r - 1)
				fmt.Printf("⎣%s"+strings.Repeat(" ", c-len(str))+"⎦\n", str)
			}
		}

	}
}
