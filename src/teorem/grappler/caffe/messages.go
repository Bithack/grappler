package caffe

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
)

/*
The interface we need to implement

type Message interface {
	Reset()
	String() string
	ProtoMessage()
}
*/

type Message struct {
	T              string
	Datum          *Datum
	NetParameter   *NetParameter
	LayerParameter *LayerParameter
	BlobProto      *BlobProto
}

func (m *Message) Unmarshal(data []byte, t string) (err error) {
	m.T = t
	switch t {
	case "Datum":
		m.Datum = new(Datum)
		err = proto.Unmarshal(data, m.Datum)
	case "NetParameter":
		m.NetParameter = new(NetParameter)
		err = proto.Unmarshal(data, m.NetParameter)
	}
	return
}

func (m *Message) MarshalText() string {
	switch m.T {
	case "Datum":
		return proto.MarshalTextString(m.Datum)
	case "NetParameter":
		return proto.MarshalTextString(m.NetParameter)
	case "LayerParameter":
		return proto.MarshalTextString(m.LayerParameter)
	}
	return ""
}

func (m *Message) UnmarshalText(data []byte, t string) (err error) {
	m.T = t
	switch t {
	case "Datum":
		m.Datum = new(Datum)
		err = proto.UnmarshalText(string(data), m.Datum)
	case "NetParameter":
		m.NetParameter = new(NetParameter)
		err = proto.UnmarshalText(string(data), m.NetParameter)
	}
	return
}

func New(T string) (f *Message) {
	f = new(Message)
	f.T = T
	switch T {
	case "NetParameter":
		f.NetParameter = new(NetParameter)
	case "LayerParameter":
		f.LayerParameter = new(LayerParameter)
	case "Datum":
		f.Datum = new(Datum)
	case "BlobProto":
		f.BlobProto = new(BlobProto)
	default:
		panic("Unknown Message type: " + T)
	}
	return
}

func (m *Message) Clone() (f *Message) {
	f = new(Message)
	f.T = m.T
	switch m.T {
	case "NetParameter":
		bts, _ := proto.Marshal(m.NetParameter)
		f.NetParameter = new(NetParameter)
		proto.Unmarshal(bts, f.NetParameter)
	case "LayerParameter":
		bts, _ := proto.Marshal(m.LayerParameter)
		f.LayerParameter = new(LayerParameter)
		proto.Unmarshal(bts, f.LayerParameter)
	case "Datum":
		bts, _ := proto.Marshal(m.Datum)
		f.Datum = new(Datum)
		proto.Unmarshal(bts, f.Datum)
	case "BlobProto":
		bts, _ := proto.Marshal(m.BlobProto)
		f.BlobProto = new(BlobProto)
		proto.Unmarshal(bts, f.BlobProto)
	}
	return
}

func (m *Message) GetField(s string) (f *Message) {
	switch m.T {
	case "LayerParameter":
		re := regexp.MustCompile("^parameter\\((\\d)\\)$")
		loc := re.FindStringSubmatch(s)
		if loc == nil {
			return
		}
		i, _ := strconv.Atoi(loc[1])
		//shape := m.LayerParameter.Blobs[i].Shape
		//dims := shape.GetDim()
		/*if len(dims) == 1 {
			f64 := make([]float64, len(m.LayerParameter.Blobs[i].Data))
			for j := range f64 {
				f64[j] = float64(m.LayerParameter.Blobs[i].Data[j])
			}
			mat := mat64.NewDense(int(dims[0]), 1, f64)
			f = variables.newVariableFromFloat(mat)
		}
		*/
		cm := new(Message)
		cm.T = "BlobProto"
		bts, _ := proto.Marshal(m.LayerParameter.Blobs[i])
		cm.BlobProto = new(BlobProto)
		proto.Unmarshal(bts, cm.BlobProto)
		f = cm

	case "NetParameter":
		for _, layer := range m.NetParameter.GetLayer() {
			if layer.GetName() == s {
				cm := new(Message)
				cm.T = "LayerParameter"
				// marshal the layer
				bts, _ := proto.Marshal(layer)
				// create a new struct
				cm.LayerParameter = new(LayerParameter)
				// unmarshal into it
				proto.Unmarshal(bts, cm.LayerParameter)
				f = cm
				break
			}
		}
	}
	return
}

func (m *Message) GetFields() (s []string) {
	switch m.T {
	case "NetParameter":
		for _, layer := range m.NetParameter.GetLayer() {
			s = append(s, *layer.Name)
			for i := 0; i < len(layer.Param); i++ {
				s = append(s, fmt.Sprintf("%v.parameter(%v)", *layer.Name, i))
			}
		}
	}
	return
}

// The conventional blob dimensions for batches of image data are number N x channel K x height H x width W.
// Blob memory is row-major in layout, so the last / rightmost dimension changes fastest.
// For example, in a 4D blob, the value at index (n, k, h, w) is physically located at index ((n * K + k) * H + h) * W + w.

func (m *Message) Print() {
	switch m.T {
	case "BlobProto":
		shape := m.BlobProto.Shape
		dims := shape.GetDim()
		if len(dims) == 1 {
			f64 := make([]float64, len(m.BlobProto.Data))
			for j := range f64 {
				f64[j] = float64(m.BlobProto.Data[j])
			}
			//mat := mat64.NewDense(int(dims[0]), 1, f64)
			//f := variables.newVariableFromFloat(mat)
			//f.Print("ans")
		}
		if len(dims) == 4 {
			di := 0
			for f := 0; f < int(dims[0]); f++ {
				fmt.Printf("Filter %v:\n", f)
				for d := 0; d < int(dims[1]); d++ {
					for r := 0; r < int(dims[2]); r++ {
						for c := 0; c < int(dims[3]); c++ {
							fl := m.BlobProto.Data[di]
							str := strconv.FormatFloat(float64(fl), 'f', 4, 64)
							fmt.Printf(str + strings.Repeat(" ", 10-len(str)))
							di++
						}
						fmt.Printf("\n")
					}
					fmt.Printf("\n")
				}
				fmt.Printf("\n")
			}
		}

	case "Datum":
		fmt.Printf("%+v", *m.Datum)

	case "LayerParameter":
		tops := m.LayerParameter.GetTop()
		bottoms := m.LayerParameter.GetBottom()
		fmt.Printf("%v - %v layer\n    Bottoms: %v\n    Tops: %v\n", m.LayerParameter.GetName(), m.LayerParameter.GetType(), bottoms, tops)
		for i, blob := range m.LayerParameter.GetBlobs() {
			shape := blob.GetShape()
			fmt.Printf("    parameter %v : %v\n", i, shape.GetDim())
		}
		var s string
		switch m.LayerParameter.GetType() {
		case "InnerProduct":

		case "Convolution":
			convParam := m.LayerParameter.GetConvolutionParam()
			s = proto.MarshalTextString(convParam)
			params := m.LayerParameter.GetParam()
			for _, p := range params {
				s = s + proto.MarshalTextString(p)
			}

		case "Data":
			param := m.LayerParameter.GetDataParam()
			s = proto.MarshalTextString(param)
		}
		fmt.Printf(s)

	case "NetParameter":
		name := m.NetParameter.GetName()
		if name != "" {
			fmt.Printf("Name: %v\n", name)
		}
		fmt.Printf("Layers:\n")
		for _, layer := range m.NetParameter.GetLayer() {
			fmt.Printf("    %v - %v\n", layer.GetType(), layer.GetName())
		}

	}
}
