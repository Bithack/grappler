package matchar

import (
	"math"

	"github.com/gonum/matrix/mat64"
)

// Matchar is a char matrix representation. Row count equals number of strings stored. Col count equals the max length of the strings.
type Matchar struct {
	mat  []string
	rows int
}

// NewMatchar creates a new matrix of type Matchar from a string slice.
func NewMatchar(mat []string) *Matchar {
	r := len(mat)
	if mat == nil {
		mat = make([]string, r)
	}
	return &Matchar{
		mat:  mat,
		rows: r,
	}
}

// Reset clears the matrix
func (m *Matchar) Reset() {
	m.mat = make([]string, 0)
	m.rows = 0
}

// Append extens the matrix with a string row (at the end)
func (m *Matchar) Append(s string) {
	m.mat = append(m.mat, s)
	m.rows++
}

// RowView returns a row
func (m *Matchar) RowView(i int) string {
	return m.mat[i]
}

// Dims returns the number of rows and columns in the matrix.
func (m *Matchar) Dims() (r, c int) {
	r = m.rows
	var cf float64
	for i := range m.mat {
		cf = math.Max(cf, float64(len(m.mat[i])))
	}
	c = int(cf)
	return
}

// T returns transpose of char matrix
func (m *Matchar) T() (mat mat64.Matrix) {
	return
}

// At returns char at position i, j
func (m *Matchar) At(i, j int) (v float64) {
	return
}
