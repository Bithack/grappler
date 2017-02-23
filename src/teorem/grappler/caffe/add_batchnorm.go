package caffe

import "github.com/golang/protobuf/proto"

func AddBatchNorm(model *Message) (newModel *Message) {

	newModel = New("NetParameter")
	newModel.NetParameter = new(NetParameter)

	layers := model.NetParameter.GetLayer()
	for _, layer := range layers {

		newLayer := proto.Clone(layer).(*LayerParameter)
		newModel.NetParameter.Layer = append(newModel.NetParameter.Layer, newLayer)

		switch *layer.Type {
		case "Convolution":
			// insert BatchNorm layer after every convolution layers
			var bn LayerParameter
			bn.Type = proto.String("BatchNorm")
			bn.Name = proto.String(*layer.Name + "_BN")
			bn.Bottom = []string{layer.Top[0]}
			bn.Top = []string{layer.Top[0]}
			bn.BatchNormParam = new(BatchNormParameter)
			newModel.NetParameter.Layer = append(newModel.NetParameter.Layer, &bn)
		}
	}
	return
}
