// MultiMatrix is a wrapper for several 2D float and char matrixes
// It implements the Matrix interface

package multimatrix

import (
	"fmt"
	"strings"

	"teorem/multimatrix/matchar"

	"github.com/gonum/matrix/mat64"
)

// MatrixType ...
type MatrixType uint8

// types
const (
	NUMERIC MatrixType = 0
	CHAR               = 1
	LOGIC              = 2
)

// MultiMatrix is the struct
type MultiMatrix struct {
	identity MatrixType
	numeric  *mat64.Dense
	char     *matchar.Matchar
}

// NewNumericFromFloats returns a new instance of a NUMERIC matrix initialized with float data slice
func NewNumericFromFloats(i, j int, data []float64) (m *MultiMatrix) {
	m.numeric = mat64.NewDense(i, j, data)
	return
}

// NewNumeric returns a new empty instance of a NUMERIC matrix
func NewNumeric(i, j int) (m *MultiMatrix) {
	m.numeric = mat64.NewDense(i, j, nil)
	return
}

// NewCharFromStrings returns a new instance of a CHAR matrix initialized with a string slice
func NewCharFromStrings(data []string) (m *MultiMatrix) {
	m.char = matchar.NewMatchar(data)
	return
}

// Dims returns the matrix dimensions
func (m *MultiMatrix) Dims() (r, c int) {
	switch m.identity {
	case NUMERIC:
		return m.numeric.Dims()
	case CHAR:
		return m.char.Dims()
	}
	return
}

// At ...
func (m *MultiMatrix) At(i, j int) (v float64) {
	switch m.identity {
	case NUMERIC:
		return m.numeric.At(i, j)
	case CHAR:
		return m.char.At(i, j)
	}
	return
}

// T ...
func (m *MultiMatrix) T() (mat mat64.Matrix) {
	switch m.identity {
	case NUMERIC:
		return m.numeric.T()
	case CHAR:
		return m.char.T()
	}
	return nil
}

// Print ...
func (m *MultiMatrix) Print(name string) {
	switch m.identity {
	case NUMERIC:
		r, c := m.Dims()
		switch {
		case r == 0 && c == 0:
			fmt.Printf("%s = []\n", name)
		case r == 1 && c == 1:
			fmt.Printf("%s = %.4f\n", name, m.numeric.At(0, 0))
		default:
			fa := mat64.Formatted(m.numeric, mat64.Excerpt(5))
			///mat64.Format(matrixes[mat], )
			fmt.Printf("%s =\n%.4f\n", name, fa)
		}

	case CHAR:
		r, c := m.Dims()
		switch {
		case r == 0 && c == 0:
			fmt.Printf("%s = []\n", name)
		default:
			fmt.Printf("%s =\n", name)
			if r > 10 {
				fmt.Printf("Dims(%v, 1)\n", r)
				str := m.char.RowView(0)
				fmt.Printf("⎡%s"+strings.Repeat(" ", c-len(str))+"⎤\n", str)
				for i := 1; i < 5; i++ {
					str = m.char.RowView(i)
					fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
				}
				fmt.Printf(" .\n .\n .\n")
				for i := r - 5; i < r-1; i++ {
					str = m.char.RowView(i)
					fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
				}
				str = m.char.RowView(r - 1)
				fmt.Printf("⎣%s"+strings.Repeat(" ", c-len(str))+"⎦\n", str)

			} else {
				str := m.char.RowView(0)
				fmt.Printf("⎡%s"+strings.Repeat(" ", c-len(str))+"⎤\n", str)
				for i := 1; i < r-1; i++ {
					str = m.char.RowView(i)
					fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
				}
				str = m.char.RowView(r - 1)
				fmt.Printf("⎣%s"+strings.Repeat(" ", c-len(str))+"⎦\n", str)
			}
			//fmt.Printf("%s =\n%.4f\n", mat, fa)
		}
	}
}
