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
	"strings"

	"strconv"

	"github.com/gonum/matrix/mat64"
	"github.com/gonum/stat"
)

var functions = []string{"pca", "mean", "max", "min", "mul", "size", "bh_tsne", "random", "rand"}

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
func parseFunctionCall(f string, args string) (result *mat64.Dense, err error) {

	if debugMode {
		fmt.Printf("parseFunctionCall %s\n", f)
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
	case "rand", "random":
		if !isScalar(argv2[0]) || (len(argv) == 2 && !isScalar(argv2[1])) {
			return nil, errors.New("random expects scalar values as parameters")
		}
		r := int(math.Floor(getScalar(argv2[0])))
		c := 1
		if len(argv) == 2 {
			c = int(math.Floor(getScalar(argv2[1])))
		}
		floats := make([]float64, r*c)
		for i := range floats {
			floats[i] = rand.Float64()
		}
		result = mat64.NewDense(r, c, floats)

	case "bh_tsne":
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
		if len(argv2) != 1 {
			return nil, errors.New("expected one arguments to size(X)")
		}
		r, c := argv2[0].Dims()
		result = mat64.NewDense(1, 2, []float64{float64(r), float64(c)})

	case "mul":
		r, _ := argv2[0].Dims()
		_, c2 := argv2[1].Dims()
		result = mat64.NewDense(r, c2, nil)
		result.Mul(argv2[0], argv2[1])

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
		matrixes["vars"] = v

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

var tempMatrixes = make(map[string]*mat64.Dense)
var tempIndex int

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
				return nil, 0, 0, errors.New("Dimension mismatch in matrix addition")
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

func parseExpression(expr string) (result *mat64.Dense, err error) {

	if debugMode {
		fmt.Printf("parseExpression %s\n", expr)
	}

	//remove all spaces from string
	expr = strings.Replace(expr, " ", "", -1)

	// ATOMS

	//simple numeric value, "3.1415"
	f, err := strconv.ParseFloat(expr, 32)
	if err == nil {
		//convert to 1x1 matrix
		a := mat64.NewDense(1, 1, []float64{f})
		return a, nil
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
			return v, nil
		}
	}

	// PARENTHESIS

	//find first inner parenthesis group
	re := regexp.MustCompile("\\([^\\)\\(]*\\)")
	loc := re.FindStringIndex(expr)
	if loc != nil {
		if loc[0] == 0 || strings.Contains("+-*/,(", string(expr[loc[0]-1])) {
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

	// FUNCTION CALLS

	//find first inner function call, "random(expr,expr,expr)"
	//any parenthesis groups within the arguments should be cleared by now :)
	for _, f := range functions {
		re := regexp.MustCompile(f + "\\([^\\)\\(]*\\)")
		loc := re.FindStringIndex(expr)
		if loc != nil {
			//The match itself is at s[loc[0]:loc[1]].
			if debugMode {
				fmt.Printf("Found function call: %s\n", expr[loc[0]:loc[1]])
			}
			name := "term_" + strconv.Itoa(tempIndex)
			tempIndex++
			tempMatrixes[name], err = parseFunctionCall(f, expr[loc[0]+len(f)+1:loc[1]-1])
			if err != nil {
				return nil, err
			}
			return parseExpression(expr[0:loc[0]] + name + expr[loc[1]:])
		}
	}

	// OPERATORS

	//split by + groups. if more than 1: parse them, check if dimensions match, then add it together
	terms, r, c, err := splitAndCheckEqualDim(expr, "+")
	if err != nil {
		return nil, err
	}
	if terms != nil {
		result = mat64.NewDense(r, c, nil)
		for i := range terms {
			result.Add(result, terms[i])
		}
		return result, nil
	}

	//split by - groups. if more than 1: parse them, check if dimensions match, then add it together
	terms, r, c, err = splitAndCheckEqualDim(expr, "-")
	if err != nil {
		return nil, err
	}
	if terms != nil {
		result = mat64.NewDense(r, c, terms[0].RawMatrix().Data)
		for i := 1; i < len(terms); i++ {
			result.Sub(result, terms[i])
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
		fa := mat64.Formatted(matrixes[mat], mat64.Excerpt(5))
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
