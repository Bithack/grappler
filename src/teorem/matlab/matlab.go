package matlab

import (
	"fmt"
	"os"
	"teorem/multimatrix"
)

type matlab5Header struct {
	headerText          [116]byte
	subsystemDataOffset [8]byte
	version             [2]byte
	endianIndicator     [2]byte
}

type matlab5DataElement struct {
	dataType      int32
	numberOfBytes int32
	data          []byte
}

// WriteMatlabFile saves the given multimatrixes in a MATLAB 5.0 file
// https://www.mathworks.com/help/pdf_doc/matlab/matfile_format.pdf
func WriteMatlabFile(filename string, mat []*multimatrix.MultiMatrix) (err error) {
	var f *os.File
	f, err = os.Create(filename)
	if err != nil {
		fmt.Printf("couldn't create file %s\n", filename)
		return
	}
	f.Close()
	return
}
