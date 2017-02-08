package main

import (
	"math"

	"github.com/gonum/floats"
	"github.com/gonum/matrix/mat64"
)

// mean computes the mean value of every column in matrix mat
func mean(mat *mat64.Dense) (result *mat64.Dense) {
	r, c := mat.Dims()
	m2 := mat64.DenseCopyOf(mat.T())
	result = mat64.NewDense(1, c, nil)
	for i := 0; i < c; i++ {
		result.Set(0, i, floats.Sum(m2.RawRowView(i))/float64(r))
	}
	return
}

// euclidean computes the euclidean distance between two vectors
func euclidean(u []float64, v []float64) (r float64) {
	for i := range u {
		a := (u[i] - v[i])
		r += a * a
	}
	return math.Sqrt(r)
}

// pdist computes pair-wise distances between every row vector in mat
func pdist(mat *mat64.Dense) (result *mat64.Dense) {
	r, _ := mat.Dims()
	result = mat64.NewDense(r*(r-1)/2, 1, nil)
	c := 0
	for i := 0; i < r; i++ {
		for j := i + 1; j < r; j++ {
			result.Set(c, 0, euclidean(mat.RawRowView(i), mat.RawRowView(j)))
			c++
		}
	}
	return
}
