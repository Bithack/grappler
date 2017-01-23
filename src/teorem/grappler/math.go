package main

import (
	"github.com/gonum/floats"
	"github.com/gonum/matrix/mat64"
)

func mean(mat *mat64.Dense) (result *mat64.Dense) {
	r, c := mat.Dims()
	m2 := mat64.DenseCopyOf(mat.T())
	result = mat64.NewDense(1, c, nil)
	for i := 0; i < c; i++ {
		//fmt.Printf("%v\n", m2.)
		result.Set(0, i, floats.Sum(m2.RawRowView(i))/float64(r))
	}
	return
}
