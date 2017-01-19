# Teorem Grappler

Grappler is an interactive multi-purpose tool for extracting keys and floating point data from key-value databases like LMDB or plain ascii files, and then working with them in a matlab-like interface.

The processed data can be saved in ascii files or bulk written to another database (only Aerospike supported so far).

We use it for extraction multi-dimensional feature vectors LMDB databases generated with the deep learning framework Caffe.

## Dependencies

Grappler needs the go compiler and toolkit. Go packages are automatically downloaded when the program is built.

In order the use tsne reduction, the binary "bh_tsne" needs to be in the path. Clone it from https://github.com/lvdmaaten/bhtsne and compile it.

## Installing, building and running it

    git clone https://github.com/Bithack/grappler.git
    cd grappler
    ./grappler.sh

For a command listning, type HELP or ? at the prompt

## Authors

* Oscar Franzén <oscar.franzen@teorem.se>

## License

Copyright (c) 2017 Teorem AB

Grappler is free and licensed under the MIT license.
