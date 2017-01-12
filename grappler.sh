#!/bin/bash
. env.sh
go get teorem/grappler && exec bin/grappler $*
