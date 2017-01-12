Teorem Grappler
===========================

Grappler is a multi-purpose tool for extracting data from key-value databases like LMDB and working with them in a matlab-like interface.
We use it for extracting floating point data from LMDB databases generated with the deep learning framework Caffe. 
Grappler can do dimensional reduction with PCA or tsne and write data to ascii files.

Dependencies
================

Grappler needs to go compiler and toolkit. Golang packages are automatically downloaded when the program is built.

In order the use the tsne reduction, the binary "bh_tsne" needs to be in the path. Clone it from https://github.com/lvdmaaten/bhtsne and compile.

Building and running it
================

    ./grappler.sh