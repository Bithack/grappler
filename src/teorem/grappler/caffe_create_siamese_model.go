package main

import (
	"teorem/grappler/caffe"

	"github.com/golang/protobuf/proto"
)

func caffeCreateSiameseModel(model *caffeMessage) (newModel *caffeMessage) {

	newModel = model.Clone()

	// add parameter sharing and clone layers
	layers := newModel.NetParameter.GetLayer()
	for _, layer := range layers {
		switch *layer.Type {
		case "Input", "Data", "ImageData", "MemoryData", "HDF5Data":
			continue // don't clone input layers

		case "Convolution", "InnerProduct":
			// set parameter sharing (weights & bias) for convolutional and innerproduct
			// overwrites any existing parameter names
			if len(layer.Param) == 2 {
				layer.Param[0].Name = newString(*layer.Name + "_w")
				layer.Param[1].Name = newString(*layer.Name + "_b")
			} else {
				ps := []*caffe.ParamSpec{new(caffe.ParamSpec), new(caffe.ParamSpec)}
				ps[0].Name = newString(*layer.Name + "_w")
				ps[1].Name = newString(*layer.Name + "_b")
				layer.Param = ps
			}
		}
		newLayer := proto.Clone(layer).(*caffe.LayerParameter)
		*newLayer.Name = *newLayer.Name + "_R"
		// add _R to tops & bottoms
		for i := range newLayer.Top {
			newLayer.Top[i] = newLayer.Top[i] + "_R"
		}
		for i := range newLayer.Bottom {
			newLayer.Bottom[i] = newLayer.Bottom[i] + "_R"
		}
		newModel.NetParameter.Layer = append(newModel.NetParameter.Layer, newLayer)
	}
	// add loss layer
	var lossLayer caffe.LayerParameter
	lossLayer.Type = newString("ContrastiveLoss")
	lossLayer.Name = newString("loss")
	last := layers[len(layers)-1].Top[0]
	lossLayer.Bottom = []string{last, last + "_R", "label"}
	lossLayer.Top = []string{"loss"}
	lossLayer.ContrastiveLossParam = new(caffe.ContrastiveLossParameter)
	lossLayer.ContrastiveLossParam.Margin = newFloat32(1)
	newModel.NetParameter.Layer = append(newModel.NetParameter.Layer, &lossLayer)

	return newModel
}
