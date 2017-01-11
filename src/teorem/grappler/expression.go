package main

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"strconv"

	"github.com/gonum/matrix/mat64"
	"github.com/gonum/stat"
)

var functions = []string{"pca", "mean", "max", "min", "mul"}

func parseFunctionCall(f string, args string) (result *mat64.Dense, err error) {

	argv := strings.Split(args, ",")
	argv2 := make([]*mat64.Dense, len(argv))
	for i := range argv {
		argv2[i], err = parseExpression(argv[i])
		if err != nil {
			return nil, errors.New("Invalid argument to " + f + "(): " + err.Error())
		}
	}
	switch f {

	case "mul":
		var a = argv2[0]
		var result mat64.Dense
		for i, v := range argv2 {
			if i > 0 {
				result.Mul(a, v)
			}
		}
		//result = mat64.DenseCopyOf(a)

	case "pca":
		if len(argv2) != 2 {
			return nil, errors.New("expected two arguments to pca(X, DIM)")
		}
		r, c := argv2[1].Dims()
		if r != 1 || c != 1 {
			return nil, errors.New("expected scalar as second argument to pca(X, DIM)")
		}
		// new DIM k
		k := int(math.Floor(argv2[1].At(0, 0)))

		//PCA wants observations in rows, variables in columns!
		vecs, vars, ok := stat.PrincipalComponents(argv2[0], nil)
		if !ok {
			return nil, errors.New("internal error in pca()")
		}
		//vecs should now be a matrix of size D x D
		r2, c2 := vecs.Dims()
		fmt.Printf("Vecs has dimensions %v, %v\n", r2, c2)

		//save variances as a row vector
		v := mat64.NewDense(1, len(vars), vars)
		matrixes["vars"] = v

		//project data
		var a mat64.Dense
		a.Mul(argv2[0], vecs.View(0, 0, r2, k))
		result = &a

		//fmt.Printf("variances = %.4f\n\n", vars)
		//k := 2
		//var proj mat64.Dense
		//proj.Mul(iris, vecs.View(0, 0, d, k))

	case "mean":
		//mean of matrix calculates returns row vector with means for ever column
		//first calc transpose matrix and then use RawRowView to get the float64 values
		if len(argv2) != 1 {
			return nil, errors.New("mean() only accepts one matrix argument")
		}
		_, c := argv2[0].Dims()
		a := mat64.DenseCopyOf(argv2[0].T())
		result = mat64.NewDense(1, c, nil)
		for i := 0; i < c; i++ {
			result.Set(0, i, stat.Mean(a.RawRowView(i), nil))
		}

	case "min":
		var min = math.MaxFloat64
		for _, v := range argv2 {
			min = math.Min(min, mat64.Min(v))
		}
		result = mat64.NewDense(1, 1, []float64{min})
	case "max":
		var max = math.SmallestNonzeroFloat64
		for _, v := range argv2 {
			max = math.Max(max, mat64.Max(v))
		}
		result = mat64.NewDense(1, 1, []float64{max})
	default:
		err = errors.New("Unknown function " + f + "()")
	}
	return
}

func parseExpression(expr string) (result *mat64.Dense, err error) {

	//remove all spaces from string
	expr = strings.Replace(expr, " ", "", -1)

	//scan for function(args) and recurs
	for _, f := range functions {
		i0 := strings.Index(expr, f+"(")
		i1 := strings.LastIndex(expr, ")")
		if i0 != -1 && i1 != -1 {
			// for now just return the result of the first function call found
			// we could add more functionality later on if needed
			return parseFunctionCall(f, expr[i0+len(f)+1:i1])
		}
		//re := regexp.MustCompile(f + "\\((?>[^()]+|(?1))*\\)")
		//f + "\\([^\\)]*\\)"
		//f + "\\((?>[^()]+|(?1))*\\)"
		//loc := re.FindStringIndex(expr)
	}

	//check for simple numeric value
	f, err := strconv.ParseFloat(expr, 32)
	if err == nil {
		//convert to 1x1 matrix
		a := mat64.NewDense(1, 1, []float64{f})
		return a, nil
	}

	//transpose
	transpose := false
	if expr[len(expr)-1:] == "'" {
		expr = expr[:len(expr)-1]
		//fmt.Printf("test: %v", expr[len(expr)-1:])
		transpose = true
	}

	//check if it is the name of an matrix
	_, ok := matrixes[expr]
	if ok {
		if transpose {
			a := mat64.DenseCopyOf(matrixes[expr].T())
			return a, nil
		}
		return matrixes[expr], nil
	}
	return nil, errors.New("Unknown expression")
}

func printMatrix(mat string) {
	r, c := matrixes[mat].Dims()
	switch {
	case r == 0 && c == 0:
		fmt.Printf("%s = []\n", mat)
	case r == 1 && c == 1:
		fmt.Printf("%s = %.4f\n", mat, matrixes[mat].At(0, 0))
	default:
		fa := mat64.Formatted(matrixes[mat], mat64.Excerpt(5))
		///mat64.Format(matrixes[mat], )
		fmt.Printf("%s =\n%.4f\n", mat, fa)
	}

}
