package main

import (
	"fmt"
	"teorem/grappler/caffe"

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

type caffeMessage struct {
	T              string
	Datum          *caffe.Datum
	NetParameter   *caffe.NetParameter
	LayerParameter *caffe.LayerParameter
}

func (m *caffeMessage) Unmarshal(data []byte, t string) (err error) {
	m.T = t
	switch t {
	case "Datum":
		m.Datum = new(caffe.Datum)
		err = proto.Unmarshal(data, m.Datum)
	case "NetParameter":
		m.NetParameter = new(caffe.NetParameter)
		err = proto.Unmarshal(data, m.NetParameter)
	}
	return
}

func (m *caffeMessage) MarshalText() string {
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

func (m *caffeMessage) UnmarshalText(data []byte, t string) (err error) {
	m.T = t
	switch t {
	case "Datum":
		m.Datum = new(caffe.Datum)
		err = proto.UnmarshalText(string(data), m.Datum)
	case "NetParameter":
		m.NetParameter = new(caffe.NetParameter)
		err = proto.UnmarshalText(string(data), m.NetParameter)
	}
	return
}

func (m *caffeMessage) Clone() (f *caffeMessage) {
	f = new(caffeMessage)
	f.T = m.T
	switch m.T {
	case "NetParameter":
		bts, _ := proto.Marshal(m.NetParameter)
		f.NetParameter = new(caffe.NetParameter)
		proto.Unmarshal(bts, f.NetParameter)
	case "LayerParameter":
		bts, _ := proto.Marshal(m.LayerParameter)
		f.LayerParameter = new(caffe.LayerParameter)
		proto.Unmarshal(bts, f.LayerParameter)
	case "Datum":
		bts, _ := proto.Marshal(m.Datum)
		f.Datum = new(caffe.Datum)
		proto.Unmarshal(bts, f.Datum)
	}
	return
}

func (m *caffeMessage) GetField(s string) (f *caffeMessage) {
	switch m.T {
	case "NetParameter":
		for _, layer := range m.NetParameter.GetLayer() {
			if layer.GetName() == s {
				f = new(caffeMessage)
				f.T = "LayerParameter"
				// marshal the layer
				bts, _ := proto.Marshal(layer)
				// create a new struct
				f.LayerParameter = new(caffe.LayerParameter)
				// unmarshal into it
				proto.Unmarshal(bts, f.LayerParameter)
				break
			}
		}
	}
	return
}

func (m *caffeMessage) GetFields() (s []string) {
	switch m.T {
	case "NetParameter":
		for _, layer := range m.NetParameter.GetLayer() {
			s = append(s, layer.GetName())
		}
	}
	return
}

func (m *caffeMessage) Print() {
	switch m.T {
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
