/*
 * // Copyright (C) 2017 yeecall authors
 * //
 * // This file is part of the yeecall library.
 *
 */

package fec

/*
#cgo CFLAGS: -I./libRaptorQ
#cgo LDFLAGS: -L/usr/local/lib
#define USE_NUM_NONE
#define USE_FIELD_10X26
#define USE_FIELD_INV_BUILTIN
#define USE_SCALAR_8X32
#define USE_SCALAR_INV_BUILTIN
#define NDEBUG
#include "./libRaptorQ/cRaptorQ.h"
#include "./libRaptorQ/cRaptorQ.cpp"
#include "./test.c"
*/
import "C"


func RaptorQ() {
   C.test()
}