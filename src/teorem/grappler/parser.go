package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"strconv"

	"github.com/gonum/matrix"
	"github.com/gonum/matrix/mat64"
	"github.com/gonum/stat"
)

var tempMatrixes = make(map[string]*mat64.Dense)
var tempIndex int
var returnMatrixes = make(map[string]*mat64.Dense)
var returnMatrixesOrder = make([]string, 0)

var limitLow, limitHigh int

var helpTexts = map[string]string{
	"pca": `pca(X, DIM) - Performs a principal components analysis on matrix X which is represented as an n×d matrix 
where each row is an observation and each column is a variable. It returns X projected down to DIM dimensions.`,
	"sum":   `sum(X, DIM) - Sum of elements along dimension DIM.`,
	"normr": `normr(X) - Normalizes X by dividing every row with the L2 norm`,
	"sort":  `sort(X, DIM) - Sorts X along dimension DIM`,
	"var":   `var(X) - Calculates variances of X per column as sum( (x_i - mean(X))^2 ) / (n-1)`,
}

func parseGetHelp(function string) (m string) {
	m, ok := helpTexts[function]
	if !ok {
		return "No such function"
	}
	return
}

func addReturn(s string, m *mat64.Dense) {
	returnMatrixes[s] = m
	returnMatrixesOrder = append(returnMatrixesOrder, s)
}

func rows(a *mat64.Dense) (r int) {
	r, _ = a.Dims()
	return
}

func cols(a *mat64.Dense) (c int) {
	_, c = a.Dims()
	return
}

func newScalar(v float64) *mat64.Dense {
	return mat64.NewDense(1, 1, []float64{float64(v)})
}
func getScalar(a *mat64.Dense) float64 {
	return a.At(0, 0)
}

func isScalar(a *mat64.Dense) bool {
	return checkDims(a, 1, 1)
}

func checkDims(a *mat64.Dense, r, c int) bool {
	r2, c2 := a.Dims()
	if r2 != r || c2 != c {
		return false
	}
	return true
}

func parseMatrixSubindex(mat *mat64.Dense, args string) (result *mat64.Dense, err error) {
	grLog(fmt.Sprintf("parseMatrixSubIndex %v", args))
	argv := strings.Split(args, ",")
	switch len(argv) {
	case 1:
		//linear indexing
		if argv[0] == ":" {
			// Return whole matrix as one column
			r, c := mat.Dims()
			result = mat64.NewDense(r*c, 1, mat.RawMatrix().Data)
			return
		}
		var a2 *mat64.Dense
		a2, err = parseExpression(argv[0])
		if err != nil {
			return nil, err
		}
		if isScalar(a2) {
			// Single index returns a single value
			index := int(getScalar(a2))
			d := mat.RawMatrix().Data
			if index < 0 || index >= len(d) {
				return nil, errors.New("Index exceeds matrix dimensions")
			}
			result = mat64.NewDense(1, 1, []float64{d[index]})
		}
		r, c := a2.Dims()
		// any matrix with valid indexes, reuse a2 for storing values
		d := mat.RawMatrix().Data
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				index := int(a2.At(i, j))
				if index < 0 || index >= len(d) {
					return nil, errors.New("Index exceeds matrix dimensions")
				}
				a2.Set(i, j, d[index])
			}
		}
		result = a2

	case 2:
		r, c := mat.Dims()
		limitLow = 0
		limitHigh = r - 1
		a0, err := parseExpression(argv[0])
		if err != nil {
			return nil, err
		}
		limitHigh = c - 1
		a1, err := parseExpression(argv[1])
		if err != nil {
			return nil, err
		}
		if rows(a0) != 1 || rows(a1) != 1 {
			return nil, errors.New("Two-dimensinal matrix indexing only accepts vectors as argument")
		}
		result = mat64.NewDense(cols(a0), cols(a1), nil)
		for i := 0; i < cols(a0); i++ {
			for j := 0; j < cols(a1); j++ {
				y := int(round(a0.At(0, i)))
				x := int(round(a1.At(0, j)))
				if y < 0 || y >= r || x < 0 || x >= c {
					return nil, errors.New("Index exceeds matrix dimensions")
				}
				result.Set(i, j, mat.At(y, x))
			}
		}
		return result, nil

	default:
		return nil, errors.New("To many variables for matrix subindexing")
	}
	return
}

func checkArguments(fname string, argv []*mat64.Dense, types []string) (err error) {
	var required int
	for j := range types {
		if strings.HasPrefix(types[j], "optional") {
			required++
		}
	}
	if len(argv) < required || len(argv) > len(types) {
		return errors.New("Incorrect number of arguments to " + fname)
	}
	for i := range argv {
		switch types[i] {
		case "dimension", "optional:dimension":
			if !isScalar(argv[i]) || (getScalar(argv[i]) != 1 && getScalar(argv[i]) != 2) {
				return errors.New("No such dimension in call to " + fname)
			}
		case "positive:integer", "optional:positive:integer":
			if !isScalar(argv[i]) || getScalar(argv[i]) != round(getScalar(argv[i])) || getScalar(argv[i]) < 1 {
				return errors.New("Expected positive integer as parameter " + strconv.Itoa(i+1) + " in call to " + fname)
			}
		case "integer", "optional:integer":
			if !isScalar(argv[i]) || getScalar(argv[i]) != round(getScalar(argv[i])) {
				return errors.New("Expected integer as parameter " + strconv.Itoa(i+1) + " in call to " + fname)
			}
		case "scalar", "optional:scalar":
			if !isScalar(argv[i]) {
				return errors.New("Expected scalar as parameter " + strconv.Itoa(i+1) + " in call to " + fname)
			}
		case "matrix", "optional:matrix":
		}
	}
	return
}

func parseFunctionCall(f string, args string) (result *mat64.Dense, err error) {

	if debugMode {
		fmt.Printf("parseFunctionCall %s\n", f)
	}

	mat, isMatrix := matrixes[f]
	if isMatrix {
		return parseMatrixSubindex(mat, args)
	}

	argv := strings.Split(args, ",")
	argv2 := make([]*mat64.Dense, len(argv))
	for i := range argv {
		argv2[i], err = parseExpression(argv[i])
		if err != nil {
			return nil, errors.New("Invalid argument to " + f + "(): " + err.Error())
		}
	}

	switch f {

	case "pdist":
		err := checkArguments("pdist(X)", argv2, []string{"matrix"})
		if err != nil {
			return nil, err
		}
		result = pdist(argv2[0])

	// a bunch of simple single valued functions
	case "exp", "sin", "sinh", "asin", "asinh", "cos", "cosh", "acos", "acosh", "tan", "tanh", "atan", "atanh", "abs", "log", "sqrt", "round", "floor", "ceil":
		if len(argv2) != 1 {
			return nil, errors.New("expected one matrix argument to " + f + "()")
		}
		r, c := argv2[0].Dims()
		result = mat64.NewDense(r, c, nil)
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				v := argv2[0].At(i, j)
				switch f {
				case "sin":
					v = math.Sin(v)
				case "sinh":
					v = math.Sinh(v)
				case "asin":
					v = math.Asin(v)
				case "asinh":
					v = math.Asinh(v)
				case "cos":
					v = math.Cos(v)
				case "cosh":
					v = math.Cosh(v)
				case "acos":
					v = math.Acos(v)
				case "acosh":
					v = math.Acosh(v)
				case "tan":
					v = math.Tan(v)
				case "tanh":
					v = math.Tanh(v)
				case "atan":
					v = math.Atan(v)
				case "atanh":
					v = math.Atanh(v)
				case "abs":
					v = math.Abs(v)
				case "log":
					v = math.Log(v)
				case "sqrt":
					v = math.Sqrt(v)
				case "floor":
					v = math.Floor(v)
				case "ceil":
					v = math.Ceil(v)
				case "exp":
					v = math.Exp(v)
				case "round":
					v = round(v)
				}
				result.Set(i, j, v)
			}
		}

		// eigenvalues
	case "eig":
		var e mat64.Eigen
		ok := e.Factorize(argv2[0], true)
		if !ok {
			return nil, errors.New("Factorization failed")
		}

		// identity matrix
	case "eye":
		if !isScalar(argv2[0]) || (len(argv) != 1) {
			return nil, errors.New("expected one scalar argument to eye()")
		}
		r := int(math.Floor(getScalar(argv2[0])))
		result = mat64.NewDense(r, r, nil)
		for i := 0; i < r; i++ {
			result.Set(i, i, 1)
		}

	case "ones":
		if !isScalar(argv2[0]) || (len(argv) == 2 && !isScalar(argv2[1])) {
			return nil, errors.New("ones() expects scalar values as parameters")
		}
		r := int(math.Floor(getScalar(argv2[0])))
		c := r
		if len(argv) == 2 {
			c = int(math.Floor(getScalar(argv2[1])))
		}
		floats := make([]float64, r*c)
		for i := range floats {
			floats[i] = 1
		}
		result = mat64.NewDense(r, c, floats)

	case "zeros":
		if !isScalar(argv2[0]) || (len(argv) == 2 && !isScalar(argv2[1])) {
			return nil, errors.New("ones() expects scalar values as parameters")
		}
		r := int(math.Floor(getScalar(argv2[0])))
		c := r
		if len(argv) == 2 {
			c = int(math.Floor(getScalar(argv2[1])))
		}
		result = mat64.NewDense(r, c, nil)

	case "rand", "random":
		if !isScalar(argv2[0]) || (len(argv) == 2 && !isScalar(argv2[1])) {
			return nil, errors.New("random expects scalar values as parameters")
		}
		r := int(math.Floor(getScalar(argv2[0])))
		c := r
		if len(argv) == 2 {
			c = int(math.Floor(getScalar(argv2[1])))
		}
		floats := make([]float64, r*c)
		for i := range floats {
			floats[i] = rand.Float64()
		}
		result = mat64.NewDense(r, c, floats)

	case "normr":
		// normr(X) normalizes the rows of X to a length of 1
		err := checkArguments("normr(X)", argv2, []string{"matrix"})
		if err != nil {
			return nil, err
		}
		r, c := argv2[0].Dims()
		for u := 0; u < r; u++ {
			a := argv2[0].RawRowView(u)
			var s float64
			for v := 0; v < c; v++ {
				s += a[v] * a[v]
			}
			s = math.Sqrt(s)
			for v := 0; v < c; v++ {
				a[v] = a[v] / s
			}
			argv2[0].SetRow(u, a)
		}
		result = argv2[0]

	case "sum":
		err := checkArguments("sum(X,dim)", argv2, []string{"matrix", "optional:dimension"})
		if err != nil {
			return nil, err
		}
		r, c := argv2[0].Dims()
		switch {
		case len(argv2) < 2 || getScalar(argv2[1]) == 1:
			// default, sum cols, return row vector
			s := mat64.NewVector(c, nil)
			for i := 0; i < r; i++ {
				s.AddVec(s, argv2[0].RowView(i))
			}
			result = mat64.NewDense(1, c, s.RawVector().Data)

		case getScalar(argv2[1]) == 2:
			// sum rows, returs col vector
			s := mat64.NewVector(r, nil)
			for i := 0; i < c; i++ {
				s.AddVec(s, argv2[0].ColView(i))
			}
			result = mat64.NewDense(r, 1, s.RawVector().Data)
		default:
			return nil, errors.New("No such dimension")
		}

	case "flip":
		err := checkArguments("flip(X,dim)", argv2, []string{"matrix", "optional:dimension"})
		if err != nil {
			return nil, err
		}
		r, c := argv2[0].Dims()
		result = mat64.NewDense(r, c, nil)
		switch {
		case len(argv2) < 2 || getScalar(argv2[1]) == 1:
			// default, flip rows
			for i := 0; i < r; i++ {
				//TODO
			}
		case getScalar(argv2[1]) == 2:
			// flip cols
			for i := 0; i < c; i++ {
				//TODO
			}
		}

	case "sort":
		err := checkArguments("sort(X,dim)", argv2, []string{"matrix", "optional:dimension"})
		if err != nil {
			return nil, err
		}
		r, c := argv2[0].Dims()
		switch {
		case (len(argv2) < 2 && r == 1) || (len(argv2) == 2 && getScalar(argv2[1]) == 2):
			// sort rows
			result = argv2[0]
			for i := 0; i < r; i++ {
				sort.Float64s(result.RawRowView(i))
			}
		default:
			// sort cols
			m2 := mat64.DenseCopyOf(argv2[0].T())
			for i := 0; i < c; i++ {
				sort.Float64s(m2.RawRowView(i))
			}
			result = mat64.DenseCopyOf(m2.T())

		}

	case "var":
		// var(X), returns the variances of every column in X
		// sum (x_i - mean(X))^2 / (n-1)
		err := checkArguments("var(X)", argv2, []string{"matrix"})
		if err != nil {
			return nil, err
		}
		r, c := argv2[0].Dims()
		mean := mean(argv2[0])
		sum := mat64.NewVector(c, nil)
		for i := 0; i < r; i++ {
			var a mat64.Vector
			a.SubVec(argv2[0].RowView(i), mean.RowView(0))
			a.MulElemVec(&a, &a)
			sum.AddVec(sum, &a)
		}
		result = mat64.NewDense(1, c, sum.RawVector().Data)
		result.Scale(1/(float64(r)-1), result)

	case "hist":
		// hist(X, n)
		// computes histograms of every column in X sorted into n bins
		// returns a matrix where row i contains the bins for column i in X
		if len(argv2) != 2 || !isScalar(argv2[1]) {
			return nil, errors.New("invalid arguments to hist(X, n)")
		}
		r, c := argv2[0].Dims()
		n := int(math.Floor(getScalar(argv2[1])))
		vmin := mat64.Min(argv2[0])
		vmax := mat64.Max(argv2[0])
		binSize := (vmax - vmin) / float64(n)
		result = mat64.NewDense(c, n, nil)
		for i := 0; i < c; i++ {
			for j := 0; j < r; j++ {
				v := argv2[0].At(j, i)
				if v == vmax {
					bin := n - 1
					result.Set(i, bin, result.At(i, bin)+1)
				} else {
					bin := int((v - vmin) / binSize)
					result.Set(i, bin, result.At(i, bin)+1)
				}
			}
		}

	case "bhtsne", "bh_tsne":
		//bh_tsne(data, no_dims, theta, perplexity)
		//
		//bh_tsne binary reads from "data.dat"
		//Binary format: n (int32), d (int32), theta (float64), perplexity (float64), dims (int32), data ([]float64)
		if len(argv2) != 4 {
			return nil, errors.New("usage: bh_tsne(data, no_dims, theta, perplexity)")
		}
		f, err := os.Create("data.dat")
		if err != nil {
			return nil, errors.New("couldn't create data file in bh_tsne")
		}
		r, c := argv2[0].Dims()

		f.Write(int32bytes(r))                                  // number of data
		f.Write(int32bytes(c))                                  // input dimensions
		f.Write(float64bytes(argv2[2].At(0, 0)))                // theta
		f.Write(float64bytes(argv2[3].At(0, 0)))                // perplexity
		f.Write(int32bytes(int(math.Floor(argv2[1].At(0, 0))))) // output dimensions
		f.Write(int32bytes(1000))                               // max iterations
		l := len(argv2[0].RawMatrix().Data)
		for i := 0; i < l; i++ {
			f.Write(float64bytes(argv2[0].RawMatrix().Data[i]))
		}
		f.Close()

		result = mat64.NewDense(1, 1, nil)

		cmd := exec.Command("bh_tsne")
		outpipe, err := cmd.StdoutPipe()
		if err != nil {
			return nil, errors.New("could not open stdoutpipe for binary bh_tsne")
		}
		err = cmd.Start()
		if err != nil {
			return nil, errors.New("could not start binary bh_tsne from bh_tsne()")
		}
		scanner := bufio.NewScanner(outpipe)
		for scanner.Scan() {
			fmt.Printf("%s\n", scanner.Text())
		}
		err = cmd.Wait()

		//read the results from result.dat
		//n (int32), d (int32), data ([]float64), landmarks ([]int32), costs ([]float64)
		f, err = os.Open("result.dat")
		if err != nil {
			return nil, errors.New("couldn't open result data file in bh_tsne")
		}
		n := readInt32(f)
		d := readInt32(f)
		data := make([]float64, n*d)
		for i := range data {
			data[i] = readFloat64(f)
		}
		f.Close()
		result = mat64.NewDense(n, d, data)
		matrixes["tsne"] = result

	case "size":
		err = checkArguments("size(X,dim)", argv2, []string{"matrix", "optional:dimension"})
		if err != nil {
			return nil, err
		}
		r, c := argv2[0].Dims()
		switch {
		case len(argv2) < 2:
			result = mat64.NewDense(1, 2, []float64{float64(r), float64(c)})
		case getScalar(argv2[1]) == 1:
			result = newScalar(float64(r))
		case getScalar(argv2[1]) == 2:
			result = newScalar(float64(c))
		default:
			return
		}

	case "mul":
		r, _ := argv2[0].Dims()
		_, c2 := argv2[1].Dims()
		result = mat64.NewDense(r, c2, nil)
		result.Mul(argv2[0], argv2[1])

	case "svd":
		if len(argv2) != 1 {
			return nil, errors.New("expected one matrix arguments to svd(X)")
		}
		r, c := argv2[0].Dims()
		var svd mat64.SVD
		svd.Factorize(argv2[0], matrix.SVDFull)
		var U, V mat64.Dense
		S := mat64.NewDense(r, c, nil)
		var values = svd.Values(nil)
		for i := range values {
			S.Set(i, i, values[i])
		}
		U.UFromSVD(&svd)
		V.VFromSVD(&svd)
		addReturn("U", &U)
		addReturn("S", S)
		addReturn("V", &V)
		result = mat64.NewDense(c, 1, values)

	case "pca":
		if len(argv2) != 2 {
			return nil, errors.New("expected two arguments to pca(X, DIM)")
		}
		r, c := argv2[1].Dims()
		if r != 1 || c != 1 {
			return nil, errors.New("expected scalar as second argument to pca(X, DIM)")
		}
		// new DIM k
		_, c = argv2[0].Dims()
		k := int(math.Floor(argv2[1].At(0, 0)))
		if k < 1 || k > c {
			return nil, errors.New("non existing DIM in pca(X, DIM)")
		}

		//PCA wants observations in rows, variables in columns!

		var pc stat.PC
		var vecs *mat64.Dense
		var vars []float64

		ok := pc.PrincipalComponents(argv2[0], nil)
		vecs = pc.Vectors(vecs)
		vars = pc.Vars(vars)

		if !ok {
			return nil, errors.New("internal error in pca()")
		}
		//vecs should now be a matrix of size D x D
		r2, _ := vecs.Dims()
		//fmt.Printf("Vecs has dimensions %v, %v\n", r2, c2)

		//save variances as a row vector
		v := mat64.NewDense(1, len(vars), vars)
		addReturn("vars", v)
		addReturn("vectors", vecs)

		//project data
		var a mat64.Dense
		a.Mul(argv2[0], vecs.View(0, 0, r2, k))
		matrixes["proj"] = &a
		result = &a

		//fmt.Printf("variances = %.4f\n\n", vars)
		//k := 2
		//var proj mat64.Dense
		//proj.Mul(iris, vecs.View(0, 0, d, k))

	case "mean":
		// mean of matrix calculates returns row vector with means for ever column
		// first calc transpose matrix and then use RawRowView to get the float64 values
		if len(argv2) != 1 {
			return nil, errors.New("expected one matrix argument to mean()")
		}
		result = mean(argv2[0])

	case "min":
		if len(argv2) != 1 {
			return nil, errors.New("expected one matrix argument to min()")
		}
		_, c := argv2[0].Dims()
		result = mat64.NewDense(1, c, nil)
		for i := 0; i < c; i++ {
			result.Set(0, i, mat64.Min(argv2[0].ColView(i)))
		}

	case "max":
		if len(argv2) != 1 {
			return nil, errors.New("expected one matrix argument to max()")
		}
		_, c := argv2[0].Dims()
		result = mat64.NewDense(1, c, nil)
		for i := 0; i < c; i++ {
			result.Set(0, i, mat64.Max(argv2[0].ColView(i)))
		}

	default:
		err = errors.New("Unknown function " + f + "()")
	}
	return
}

func splitAndParse(expr string, sep string, acceptEmpty bool) (terms2 []*mat64.Dense, err error) {
	grLog(fmt.Sprintf("splitAndParse %v, %v", expr, sep))
	terms := strings.Split(expr, sep)
	if len(terms) > 1 {
		terms2 = make([]*mat64.Dense, len(terms))
		for i := range terms {
			if len(terms[i]) == 0 {
				if acceptEmpty && i == 0 { //accepts first term missing (-4, +3)
					continue
				}
				return nil, errors.New("Missing term")
			}
			terms2[i], err = parseExpression(terms[i])
			if err != nil {
				return nil, err
			}
		}
	}
	return
}

func splitAndCheckEqualDim(expr string, sep string) (terms2 []*mat64.Dense, lastr int, lastc int, err error) {
	terms := strings.Split(expr, sep)
	if len(terms) > 1 {
		terms2 = make([]*mat64.Dense, len(terms))
		for i := range terms {

			if len(terms[i]) == 0 {
				//accepts first term missing (-4, +3)
				if i == 0 {
					continue
				}
				return nil, 0, 0, errors.New("Missing term")
			}
			terms2[i], err = parseExpression(terms[i])
			if err != nil {
				return nil, 0, 0, err
			}
			r, c := terms2[i].Dims()
			if (lastr != 0 && lastr != r) || (lastc != 0 && lastc != c) {
				return nil, 0, 0, errors.New("Dimension mismatch")
			}
			lastr = r
			lastc = c
		}
		if len(terms[0]) == 0 {
			terms2[0] = mat64.NewDense(lastr, lastc, nil)
		}
	}
	return
}

func clearParser() {
	grLog("Clearing memory used by parser")
	for i := range tempMatrixes {
		//tempMatrixes[i] = nil
		delete(tempMatrixes, i)
	}
	for i := range returnMatrixes {
		//returnMatrixes[i] = nil
		delete(returnMatrixes, i)
	}
	returnMatrixesOrder = returnMatrixesOrder[:0]
}

func parseColonExpression(expr string) (result *mat64.Dense, err error) {
	grLog(fmt.Sprintf("parseColonExpression %v", expr))
	parts := strings.Split(expr, ":")
	if len(parts) == 2 {
		var a, b float64
		var e1, e2 error
		if parts[0] == "" && parts[1] == "" {
			// single ":"
			a, b = float64(limitLow), float64(limitHigh)
		} else {
			a, e1 = strconv.ParseFloat(parts[0], 64)
			b, e2 = strconv.ParseFloat(parts[1], 64)
		}
		if e1 == nil && e2 == nil && a <= b {
			result = mat64.NewDense(1, int(b-a)+1, nil)
			for i := 0; i <= int(b-a); i++ {
				result.Set(0, i, a+float64(i))
			}
			return
		}
	}
	if len(parts) == 3 {
		a, e1 := strconv.ParseFloat(parts[0], 64)
		b, e2 := strconv.ParseFloat(parts[1], 64)
		c, e3 := strconv.ParseFloat(parts[2], 64)
		if e1 == nil && e2 == nil && e3 == nil && a <= c && b < (c-a) {
			result = mat64.NewDense(1, int((c-a)/b)+1, nil)
			for i := 0; i <= int((c-a)/b); i++ {
				result.Set(0, i, a+float64(i)*b)
			}
			return
		}
	}
	return nil, errors.New("Invalid colon expression")
}

func parseExpression(expr string) (result *mat64.Dense, err error) {
	grLog(fmt.Sprintf("parseExpression %s", expr))

	// trim spaces from string
	expr = strings.Trim(expr, " ")

	// ATOMS

	// matrix declaration [expr;expr;...;expr]
	if len(expr) > 1 && expr[0:1] == "[" && expr[len(expr)-1:] == "]" {
		//rows seperated with ;
		rows := strings.Split(expr[1:len(expr)-1], ";")
		var c int
		for i := range rows {
			cols := strings.Fields(rows[i])
			// cols could be scalar, or a:b, or a:b:c...
			if c == 0 {
				c = len(cols)
				result = mat64.NewDense(len(rows), c, nil)
			} else if len(cols) != c {
				return nil, errors.New("Different number of columns in matrix declaration")
			}
			for j := range cols {
				v, err := parseExpression(cols[j])
				if err != nil {
					return nil, err
				}
				if !isScalar(v) {
					return nil, errors.New("Only scalar values supported within a matrix declaration")
				}
				result.Set(i, j, getScalar(v))
			}
		}
		return
	}

	// simple numeric value, "3.1415"
	f, err := strconv.ParseFloat(expr, 64)
	if err == nil {
		//convert to 1x1 matrix
		a := mat64.NewDense(1, 1, []float64{f})
		return a, nil
	}

	// : or a:b or a:b:c where a,b,c are valid numeric values
	test := strings.Split(expr, ":")
	if len(test) == 2 || len(test) == 3 {
		var err error
		for _, s := range test {
			_, err = strconv.ParseFloat(s, 64)
			if err != nil {
				break
			}
		}
		if err == nil || (len(test) == 2 && test[0] == "" && test[1] == "") {
			return parseColonExpression(expr)
		}
	}

	if expr == "pi" {
		return mat64.NewDense(1, 1, []float64{math.Pi}), nil
	}

	//variables, with or without transpose, "term_3", term_3'", "ans'""
	if len(expr) > 1 && expr[len(expr)-1:] == "'" {
		//transpose of variables, "term_3'", "ans'""
		v, ok := tempMatrixes[expr[0:len(expr)-1]]
		if ok {
			return mat64.DenseCopyOf(v.T()), nil
		}
		v, ok = matrixes[expr[0:len(expr)-1]]
		if ok {
			return mat64.DenseCopyOf(v.T()), nil
		}
	} else {
		v, ok := tempMatrixes[expr]
		if ok {
			return v, nil
		}
		v, ok = matrixes[expr]
		if ok {
			return mat64.DenseCopyOf(v), nil
		}
	}

	// PARENTHESIS

	//find first inner parenthesis group
	re := regexp.MustCompile("\\([^\\)\\(]*\\)")
	loc := re.FindStringIndex(expr)
	if loc != nil {
		if loc[0] == 0 || strings.Contains("+-*/, (", string(expr[loc[0]-1])) {
			if debugMode {
				fmt.Printf("found parenthesis group at %v - %v\n", loc[0], loc[1])
			}
			name := "term_" + strconv.Itoa(tempIndex)
			tempIndex++
			tempMatrixes[name], err = parseExpression(expr[loc[0]+1 : loc[1]-1])
			if err != nil {
				return nil, err
			}
			return parseExpression(expr[0:loc[0]] + name + expr[loc[1]:])
		}
	}

	// FUNCTION CALLS AND MATRIX SUBINDEXING

	// find first inner function call "rand(expr,expr,expr)" or matrix subindex "A(2)", "A(1,2)", "A(:,:)"
	// any parenthesis groups within the arguments should be cleared by now

	re = regexp.MustCompile("([_a-zA-Z0-9]+)\\(([^\\)\\(]*)\\)")
	re.Longest()
	loc = re.FindStringSubmatchIndex(expr)
	if loc != nil {
		if debugMode {
			fmt.Printf("Found function call: %v(%v)\n", expr[loc[2]:loc[3]], expr[loc[4]:loc[5]])
		}
		name := "term_" + strconv.Itoa(tempIndex)
		tempIndex++
		tempMatrixes[name], err = parseFunctionCall(expr[loc[2]:loc[3]], expr[loc[4]:loc[5]])
		if err != nil {
			return nil, err
		}
		return parseExpression(expr[0:loc[0]] + name + expr[loc[1]:])
	}

	// OPERATORS

	// + and -
	for _, op := range []string{"+", "-"} {
		// split by + groups
		terms, err := splitAndParse(expr, op, true)
		if err != nil {
			return nil, err
		}
		if terms != nil {
			// initialize result to first the term, dimensions will stay the same
			r, c := terms[0].Dims()
			result = mat64.NewDense(r, c, terms[0].RawMatrix().Data)
			for i := 1; i < len(terms); i++ {
				r2, c2 := terms[i].Dims()
				switch {
				case r2 == 1 && c2 == 1:
					//scalar addition/subtraction
					for u := 0; u < r; u++ {
						for v := 0; v < c; v++ {
							switch op {
							case "+":
								result.Set(u, v, result.At(u, v)+getScalar(terms[i]))
							case "-":
								result.Set(u, v, result.At(u, v)-getScalar(terms[i]))
							}
						}
					}
				case r2 == r && c2 == c:
					// full matrix addition/subtraction
					switch op {
					case "+":
						result.Add(result, terms[i])
					case "-":
						result.Sub(result, terms[i])
					}
				case r2 == 1 && c2 == c:
					// row vector addition, add current term from every row in result
					for u := 0; u < r; u++ {
						var a mat64.Vector
						switch op {
						case "+":
							a.AddVec(result.RowView(u), terms[i].RowView(0))
						case "-":
							a.SubVec(result.RowView(u), terms[i].RowView(0))
						}
						result.SetRow(u, a.RawVector().Data)
					}
				default:
					return nil, errors.New("Dimension mismatch")
				}
			}
			return result, nil
		}
	}

	// split by ./ groups. if more than 1: parse them
	terms, _, _, err := splitAndCheckEqualDim(expr, "./")
	if err != nil {
		return nil, err
	}
	if len(terms) > 1 {
		result = mat64.DenseCopyOf(terms[0])
		for i := 1; i < len(terms); i++ {
			result.DivElem(result, terms[i])
		}
		return result, nil
	}

	// split by .* groups. if more than 1: parse them
	terms, _, _, err = splitAndCheckEqualDim(expr, ".*")
	if err != nil {
		return nil, err
	}
	if len(terms) > 1 {
		result = mat64.DenseCopyOf(terms[0])
		for i := 1; i < len(terms); i++ {
			result.MulElem(result, terms[i])
		}
		return result, nil
	}

	// split by * groups. if more than 1: parse them
	terms, err = splitAndParse(expr, "*", false)
	if err != nil {
		return nil, err
	}
	if terms != nil {
		result = terms[0]
		for i := 1; i < len(terms); i++ {
			r, c := result.Dims()
			r2, c2 := terms[i].Dims()
			switch {
			case r2 == 1 && c2 == 1:
				result.Scale(terms[i].At(0, 0), result)
			case r == 1 && c == 1:
				terms[i].Scale(result.At(0, 0), terms[i])
				result = terms[i]
			case r2 == c:
				res2 := mat64.NewDense(r, c2, nil)
				res2.Mul(result, terms[i])
				result = res2
			default:
				return nil, errors.New("Dimension mismatch")
			}
		}
		return result, nil
	}

	// split by / groups
	terms, err = splitAndParse(expr, "/", false)
	if err != nil {
		return nil, err
	}
	if terms != nil {
		result = terms[0]
		for i := 1; i < len(terms); i++ {
			r2, c2 := terms[i].Dims()
			switch {
			case r2 == 1 && c2 == 1:
				result.Scale(1/getScalar(terms[i]), result)
			default:
				return nil, errors.New("Dimension mismatch")
			}
		}
		return result, nil
	}

	// split by ' ' groups, build a new "row" matrix by concat
	terms, err = splitAndParse(expr, " ", false)
	if err != nil {
		return nil, err
	}
	if len(terms) > 1 {
		grLog(fmt.Sprintf("%v", terms))
		r, c := terms[0].Dims()
		result = mat64.NewDense(r, c*len(terms), nil)
		for i := 0; i < len(terms); i++ {
			r2, c2 := terms[i].Dims()
			if r2 != r || c2 != c {
				return nil, errors.New("Dimension mismatch")
			}
			for j := 0; j < c2; j++ {
				//brute force version, ColView().RawVector doesnt work
				for y := 0; y < r; y++ {
					result.Set(y, i*c2+j, terms[i].At(y, j))
				}
			}
		}
		return result, nil
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
		s := int(maxPrintWidth / 2)
		fa := mat64.Formatted(matrixes[mat], mat64.Excerpt(s))
		///mat64.Format(matrixes[mat], )
		fmt.Printf("%s =\n%.4f\n", mat, fa)
	}
}

func printCharMatrix(mat string) {
	r, c := matrixesChar[mat].Dims()
	switch {
	case r == 0 && c == 0:
		fmt.Printf("%s = []\n", mat)
	default:
		fmt.Printf("%s =\n", mat)
		if r > 10 {
			fmt.Printf("Dims(%v, 1)\n", r)
			str := matrixesChar[mat].RowView(0)
			fmt.Printf("⎡%s"+strings.Repeat(" ", c-len(str))+"⎤\n", str)
			for i := 1; i < 5; i++ {
				str = matrixesChar[mat].RowView(i)
				fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
			}
			fmt.Printf(" .\n .\n .\n")
			for i := r - 5; i < r-1; i++ {
				str = matrixesChar[mat].RowView(i)
				fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
			}
			str = matrixesChar[mat].RowView(r - 1)
			fmt.Printf("⎣%s"+strings.Repeat(" ", c-len(str))+"⎦\n", str)

		} else {
			str := matrixesChar[mat].RowView(0)
			fmt.Printf("⎡%s"+strings.Repeat(" ", c-len(str))+"⎤\n", str)
			for i := 1; i < r-1; i++ {
				str = matrixesChar[mat].RowView(i)
				fmt.Printf("⎢%s"+strings.Repeat(" ", c-len(str))+"⎥\n", str)
			}
			str = matrixesChar[mat].RowView(r - 1)
			fmt.Printf("⎣%s"+strings.Repeat(" ", c-len(str))+"⎦\n", str)
		}
		//fmt.Printf("%s =\n%.4f\n", mat, fa)
	}
}

func readInt32(f *os.File) int {
	bs := make([]byte, 4)
	f.Read(bs)
	return int(binary.LittleEndian.Uint32(bs))
}

func readFloat64(f *os.File) float64 {
	bs := make([]byte, 8)
	f.Read(bs)
	return math.Float64frombits(binary.LittleEndian.Uint64(bs))
}

func int32bytes(i int) (bs []byte) {
	bs = make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(i))
	return
}

func float64bytes(float float64) (bs []byte) {
	bits := math.Float64bits(float)
	bs = make([]byte, 8)
	binary.LittleEndian.PutUint64(bs, bits)
	return
}

func round(v float64) float64 {
	if v < 0 {
		return math.Ceil(v - 0.5)
	}
	return math.Floor(v + 0.5)
}
