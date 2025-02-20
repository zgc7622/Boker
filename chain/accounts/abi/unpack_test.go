// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package abi

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/Bokerchain/Boker/chain/common"
)

type unpackTest struct {
	def  string      // ABI definition JSON
	enc  string      // evm return data
	want interface{} // the expected output
	err  string      // empty or error if expected
}

func (test unpackTest) checkError(err error) error {
	if err != nil {
		if len(test.err) == 0 {
			return fmt.Errorf("expected no err but got: %v", err)
		} else if err.Error() != test.err {
			return fmt.Errorf("expected err: '%v' got err: %q", test.err, err)
		}
	} else if len(test.err) > 0 {
		return fmt.Errorf("expected err: %v but got none", test.err)
	}
	return nil
}

var unpackTests = []unpackTest{
	{
		def:  `[{ "type": "bool" }]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: true,
	},
	{
		def:  `[{"type": "uint32"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: uint32(1),
	},
	{
		def:  `[{"type": "uint32"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: uint16(0),
		err:  "abi: cannot unmarshal uint32 in to uint16",
	},
	{
		def:  `[{"type": "uint17"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: uint16(0),
		err:  "abi: cannot unmarshal *big.Int in to uint16",
	},
	{
		def:  `[{"type": "uint17"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: big.NewInt(1),
	},
	{
		def:  `[{"type": "int32"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: int32(1),
	},
	{
		def:  `[{"type": "int32"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: int16(0),
		err:  "abi: cannot unmarshal int32 in to int16",
	},
	{
		def:  `[{"type": "int17"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: int16(0),
		err:  "abi: cannot unmarshal *big.Int in to int16",
	},
	{
		def:  `[{"type": "int17"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001",
		want: big.NewInt(1),
	},
	{
		def:  `[{"type": "address"}]`,
		enc:  "0000000000000000000000000100000000000000000000000000000000000000",
		want: common.Address{1},
	},
	{
		def:  `[{"type": "bytes32"}]`,
		enc:  "0100000000000000000000000000000000000000000000000000000000000000",
		want: [32]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	},
	{
		def:  `[{"type": "bytes"}]`,
		enc:  "000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000200100000000000000000000000000000000000000000000000000000000000000",
		want: common.Hex2Bytes("0100000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		def:  `[{"type": "bytes"}]`,
		enc:  "000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000200100000000000000000000000000000000000000000000000000000000000000",
		want: [32]byte{},
		err:  "abi: cannot unmarshal []uint8 in to [32]uint8",
	},
	{
		def:  `[{"type": "bytes32"}]`,
		enc:  "000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000200100000000000000000000000000000000000000000000000000000000000000",
		want: []byte(nil),
		err:  "abi: cannot unmarshal [32]uint8 in to []uint8",
	},
	{
		def:  `[{"type": "bytes32"}]`,
		enc:  "0100000000000000000000000000000000000000000000000000000000000000",
		want: common.HexToHash("0100000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		def:  `[{"type": "function"}]`,
		enc:  "0100000000000000000000000000000000000000000000000000000000000000",
		want: [24]byte{1},
	},
	// slices
	{
		def:  `[{"type": "uint8[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []uint8{1, 2},
	},
	{
		def:  `[{"type": "uint8[2]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2]uint8{1, 2},
	},
	// multi dimensional, if these pass, all types that don't require length prefix should pass
	{
		def:  `[{"type": "uint8[][]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000E0000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [][]uint8{{1, 2}, {1, 2}},
	},
	{
		def:  `[{"type": "uint8[2][2]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2][2]uint8{{1, 2}, {1, 2}},
	},
	{
		def:  `[{"type": "uint8[][2]"}]`,
		enc:  "000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000800000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000001",
		want: [2][]uint8{{1}, {1}},
	},
	{
		def:  `[{"type": "uint8[2][]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [][2]uint8{{1, 2}},
	},
	{
		def:  `[{"type": "uint16[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []uint16{1, 2},
	},
	{
		def:  `[{"type": "uint16[2]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2]uint16{1, 2},
	},
	{
		def:  `[{"type": "uint32[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []uint32{1, 2},
	},
	{
		def:  `[{"type": "uint32[2]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2]uint32{1, 2},
	},
	{
		def:  `[{"type": "uint64[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []uint64{1, 2},
	},
	{
		def:  `[{"type": "uint64[2]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2]uint64{1, 2},
	},
	{
		def:  `[{"type": "uint256[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []*big.Int{big.NewInt(1), big.NewInt(2)},
	},
	{
		def:  `[{"type": "uint256[3]"}]`,
		enc:  "000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003",
		want: [3]*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)},
	},
	{
		def:  `[{"type": "int8[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []int8{1, 2},
	},
	{
		def:  `[{"type": "int8[2]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2]int8{1, 2},
	},
	{
		def:  `[{"type": "int16[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []int16{1, 2},
	},
	{
		def:  `[{"type": "int16[2]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2]int16{1, 2},
	},
	{
		def:  `[{"type": "int32[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []int32{1, 2},
	},
	{
		def:  `[{"type": "int32[2]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2]int32{1, 2},
	},
	{
		def:  `[{"type": "int64[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []int64{1, 2},
	},
	{
		def:  `[{"type": "int64[2]"}]`,
		enc:  "00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: [2]int64{1, 2},
	},
	{
		def:  `[{"type": "int256[]"}]`,
		enc:  "0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002",
		want: []*big.Int{big.NewInt(1), big.NewInt(2)},
	},
	{
		def:  `[{"type": "int256[3]"}]`,
		enc:  "000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003",
		want: [3]*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)},
	},
}

func TestUnpack(t *testing.T) {
	for i, test := range unpackTests {
		def := fmt.Sprintf(`[{ "name" : "method", "outputs": %s}]`, test.def)
		abi, err := JSON(strings.NewReader(def))
		if err != nil {
			t.Fatalf("invalid ABI definition %s: %v", def, err)
		}
		encb, err := hex.DecodeString(test.enc)
		if err != nil {
			t.Fatalf("invalid hex: %s" + test.enc)
		}
		outptr := reflect.New(reflect.TypeOf(test.want))
		err = abi.Unpack(outptr.Interface(), "method", encb)
		if err := test.checkError(err); err != nil {
			t.Errorf("test %d (%v) failed: %v", i, test.def, err)
			continue
		}
		out := outptr.Elem().Interface()
		if !reflect.DeepEqual(test.want, out) {
			t.Errorf("test %d (%v) failed: expected %v, got %v", i, test.def, test.want, out)
		}
	}
}

func TestMultiReturnWithStruct(t *testing.T) {
	const definition = `[
	{ "name" : "multi", "constant" : false, "outputs": [ { "name": "Int", "type": "uint256" }, { "name": "String", "type": "string" } ] }]`

	abi, err := JSON(strings.NewReader(definition))
	if err != nil {
		t.Fatal(err)
	}

	// using buff to make the code readable
	buff := new(bytes.Buffer)
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000040"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000005"))
	stringOut := "hello"
	buff.Write(common.RightPadBytes([]byte(stringOut), 32))

	var inter struct {
		Int    *big.Int
		String string
	}
	err = abi.Unpack(&inter, "multi", buff.Bytes())
	if err != nil {
		t.Error(err)
	}

	if inter.Int == nil || inter.Int.Cmp(big.NewInt(1)) != 0 {
		t.Error("expected Int to be 1 got", inter.Int)
	}

	if inter.String != stringOut {
		t.Error("expected String to be", stringOut, "got", inter.String)
	}

	var reversed struct {
		String string
		Int    *big.Int
	}

	err = abi.Unpack(&reversed, "multi", buff.Bytes())
	if err != nil {
		t.Error(err)
	}

	if reversed.Int == nil || reversed.Int.Cmp(big.NewInt(1)) != 0 {
		t.Error("expected Int to be 1 got", reversed.Int)
	}

	if reversed.String != stringOut {
		t.Error("expected String to be", stringOut, "got", reversed.String)
	}
}

func TestUnmarshal(t *testing.T) {
	const definition = `[
	{ "name" : "int", "constant" : false, "outputs": [ { "type": "uint256" } ] },
	{ "name" : "bool", "constant" : false, "outputs": [ { "type": "bool" } ] },
	{ "name" : "bytes", "constant" : false, "outputs": [ { "type": "bytes" } ] },
	{ "name" : "fixed", "constant" : false, "outputs": [ { "type": "bytes32" } ] },
	{ "name" : "multi", "constant" : false, "outputs": [ { "type": "bytes" }, { "type": "bytes" } ] },
	{ "name" : "intArraySingle", "constant" : false, "outputs": [ { "type": "uint256[3]" } ] },
	{ "name" : "addressSliceSingle", "constant" : false, "outputs": [ { "type": "address[]" } ] },
	{ "name" : "addressSliceDouble", "constant" : false, "outputs": [ { "name": "a", "type": "address[]" }, { "name": "b", "type": "address[]" } ] },
	{ "name" : "mixedBytes", "constant" : true, "outputs": [ { "name": "a", "type": "bytes" }, { "name": "b", "type": "bytes32" } ] }]`

	abi, err := JSON(strings.NewReader(definition))
	if err != nil {
		t.Fatal(err)
	}
	buff := new(bytes.Buffer)

	// marshall mixed bytes (mixedBytes)
	p0, p0Exp := []byte{}, common.Hex2Bytes("01020000000000000000")
	p1, p1Exp := [32]byte{}, common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000ddeeff")
	mixedBytes := []interface{}{&p0, &p1}

	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000040"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000ddeeff"))
	buff.Write(common.Hex2Bytes("000000000000000000000000000000000000000000000000000000000000000a"))
	buff.Write(common.Hex2Bytes("0102000000000000000000000000000000000000000000000000000000000000"))

	err = abi.Unpack(&mixedBytes, "mixedBytes", buff.Bytes())
	if err != nil {
		t.Error(err)
	} else {
		if bytes.Compare(p0, p0Exp) != 0 {
			t.Errorf("unexpected value unpacked: want %x, got %x", p0Exp, p0)
		}

		if bytes.Compare(p1[:], p1Exp) != 0 {
			t.Errorf("unexpected value unpacked: want %x, got %x", p1Exp, p1)
		}
	}

	// marshal int
	var Int *big.Int
	err = abi.Unpack(&Int, "int", common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001"))
	if err != nil {
		t.Error(err)
	}

	if Int == nil || Int.Cmp(big.NewInt(1)) != 0 {
		t.Error("expected Int to be 1 got", Int)
	}

	// marshal bool
	var Bool bool
	err = abi.Unpack(&Bool, "bool", common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001"))
	if err != nil {
		t.Error(err)
	}

	if !Bool {
		t.Error("expected Bool to be true")
	}

	// marshal dynamic bytes max length 32
	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000020"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000020"))
	bytesOut := common.RightPadBytes([]byte("hello"), 32)
	buff.Write(bytesOut)

	var Bytes []byte
	err = abi.Unpack(&Bytes, "bytes", buff.Bytes())
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(Bytes, bytesOut) {
		t.Errorf("expected %x got %x", bytesOut, Bytes)
	}

	// marshall dynamic bytes max length 64
	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000020"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000040"))
	bytesOut = common.RightPadBytes([]byte("hello"), 64)
	buff.Write(bytesOut)

	err = abi.Unpack(&Bytes, "bytes", buff.Bytes())
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(Bytes, bytesOut) {
		t.Errorf("expected %x got %x", bytesOut, Bytes)
	}

	// marshall dynamic bytes max length 64
	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000020"))
	buff.Write(common.Hex2Bytes("000000000000000000000000000000000000000000000000000000000000003f"))
	bytesOut = common.RightPadBytes([]byte("hello"), 64)
	buff.Write(bytesOut)

	err = abi.Unpack(&Bytes, "bytes", buff.Bytes())
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(Bytes, bytesOut[:len(bytesOut)-1]) {
		t.Errorf("expected %x got %x", bytesOut[:len(bytesOut)-1], Bytes)
	}

	// marshal dynamic bytes output empty
	err = abi.Unpack(&Bytes, "bytes", nil)
	if err == nil {
		t.Error("expected error")
	}

	// marshal dynamic bytes length 5
	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000020"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000005"))
	buff.Write(common.RightPadBytes([]byte("hello"), 32))

	err = abi.Unpack(&Bytes, "bytes", buff.Bytes())
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(Bytes, []byte("hello")) {
		t.Errorf("expected %x got %x", bytesOut, Bytes)
	}

	// marshal dynamic bytes length 5
	buff.Reset()
	buff.Write(common.RightPadBytes([]byte("hello"), 32))

	var hash common.Hash
	err = abi.Unpack(&hash, "fixed", buff.Bytes())
	if err != nil {
		t.Error(err)
	}

	helloHash := common.BytesToHash(common.RightPadBytes([]byte("hello"), 32))
	if hash != helloHash {
		t.Errorf("Expected %x to equal %x", hash, helloHash)
	}

	// marshal error
	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000020"))
	err = abi.Unpack(&Bytes, "bytes", buff.Bytes())
	if err == nil {
		t.Error("expected error")
	}

	err = abi.Unpack(&Bytes, "multi", make([]byte, 64))
	if err == nil {
		t.Error("expected error")
	}

	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000002"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000003"))
	// marshal int array
	var intArray [3]*big.Int
	err = abi.Unpack(&intArray, "intArraySingle", buff.Bytes())
	if err != nil {
		t.Error(err)
	}
	var testAgainstIntArray [3]*big.Int
	testAgainstIntArray[0] = big.NewInt(1)
	testAgainstIntArray[1] = big.NewInt(2)
	testAgainstIntArray[2] = big.NewInt(3)

	for i, Int := range intArray {
		if Int.Cmp(testAgainstIntArray[i]) != 0 {
			t.Errorf("expected %v, got %v", testAgainstIntArray[i], Int)
		}
	}
	// marshal address slice
	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000020")) // offset
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001")) // size
	buff.Write(common.Hex2Bytes("0000000000000000000000000100000000000000000000000000000000000000"))

	var outAddr []common.Address
	err = abi.Unpack(&outAddr, "addressSliceSingle", buff.Bytes())
	if err != nil {
		t.Fatal("didn't expect error:", err)
	}

	if len(outAddr) != 1 {
		t.Fatal("expected 1 item, got", len(outAddr))
	}

	if outAddr[0] != (common.Address{1}) {
		t.Errorf("expected %x, got %x", common.Address{1}, outAddr[0])
	}

	// marshal multiple address slice
	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000040")) // offset
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000080")) // offset
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000001")) // size
	buff.Write(common.Hex2Bytes("0000000000000000000000000100000000000000000000000000000000000000"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000002")) // size
	buff.Write(common.Hex2Bytes("0000000000000000000000000200000000000000000000000000000000000000"))
	buff.Write(common.Hex2Bytes("0000000000000000000000000300000000000000000000000000000000000000"))

	var outAddrStruct struct {
		A []common.Address
		B []common.Address
	}
	err = abi.Unpack(&outAddrStruct, "addressSliceDouble", buff.Bytes())
	if err != nil {
		t.Fatal("didn't expect error:", err)
	}

	if len(outAddrStruct.A) != 1 {
		t.Fatal("expected 1 item, got", len(outAddrStruct.A))
	}

	if outAddrStruct.A[0] != (common.Address{1}) {
		t.Errorf("expected %x, got %x", common.Address{1}, outAddrStruct.A[0])
	}

	if len(outAddrStruct.B) != 2 {
		t.Fatal("expected 1 item, got", len(outAddrStruct.B))
	}

	if outAddrStruct.B[0] != (common.Address{2}) {
		t.Errorf("expected %x, got %x", common.Address{2}, outAddrStruct.B[0])
	}
	if outAddrStruct.B[1] != (common.Address{3}) {
		t.Errorf("expected %x, got %x", common.Address{3}, outAddrStruct.B[1])
	}

	// marshal invalid address slice
	buff.Reset()
	buff.Write(common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000100"))

	err = abi.Unpack(&outAddr, "addressSliceSingle", buff.Bytes())
	if err == nil {
		t.Fatal("expected error:", err)
	}
}


//这里提供了一个函数，来对合约的abi进行解析
func DecodeAbi(abiJson string, name string, payload string) ([]string, error) {

	var outArray = make([]string, 0)
	return outArray, nil

	const definition = `[{"constant":true,"inputs":[],"name":"show","outputs":[{"name":"","type":"string"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"name":"str","type":"string"}],"name":"test","outputs":[{"name":"","type":"string"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"payable":false,"stateMutability":"nonpayable","type":"fallback"}]`

	//解析Abi格式成为Json格式
	var outArray = make([]string, 0)
	abi, err := abi.JSON(strings.NewReader(definition))
	if err != nil {
		return outArray, err
	}

	payload = "f9fbd554000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000046861686100000000000000000000000000000000000000000000000000000000"
	name = "test"
	//剔除最前面的0x标记
	var decodeString string = ""
	hexFlag := strings.Index(payload, "0x")
	if hexFlag == -1 {
		decodeString = payload
	} else {
		decodeString = payload[2:]
	}

	//将字符串转换成[]Byte
	decodeBytes, err := hex.DecodeString(decodeString)
	if err != nil {
		return outArray, err
	}
	log.Info("DecodeAbi", "decodeBytes", decodeBytes)

	//根据函数的名称，设置函数的输入参数信息
	method, ok := abi.Methods[name]
	if !ok {
		return outArray, errors.New("")
	}

	var mixedParams = make([]interface{}, 0)
	for _, v := range method.Inputs {

		kind := reflect.TypeOf(v.Type).Kind()
		switch kind {

		case reflect.Bool:
			var Bool bool
			mixedParams = append(mixedParams, Bool)
		case reflect.Int:
			var Int int
			mixedParams = append(mixedParams, Int)
		case reflect.Int8:
			var Int8 int8
			mixedParams = append(mixedParams, Int8)
		case reflect.Int16:
			var Int16 int16
			mixedParams = append(mixedParams, Int16)
		case reflect.Int32:
			var Int32 int32
			mixedParams = append(mixedParams, Int32)
		case reflect.Int64:
			var Int64 int64
			mixedParams = append(mixedParams, Int64)
		case reflect.Uint:
			var UInt uint
			mixedParams = append(mixedParams, UInt)
		case reflect.Uint8:
			var UInt8 uint8
			mixedParams = append(mixedParams, UInt8)
		case reflect.Uint16:
			var UInt16 uint16
			mixedParams = append(mixedParams, UInt16)
		case reflect.Uint32:
			var UInt32 uint32
			mixedParams = append(mixedParams, UInt32)
		case reflect.Uint64:
			var UInt64 uint64
			mixedParams = append(mixedParams, UInt64)
		case reflect.String:
			var String string
			mixedParams = append(mixedParams, String)
		}
	}

	//解析
	if err := abi.Unpack(&mixedParams, name, decodeBytes[4:]); err != nil {

		log.Info("DecodeAbi", "err", err)
		return nil, err
	}
	log.Info("DecodeAbi", "mixedParams", mixedParams[0])
	return outArray, nil*/
}


/*func main() {
	
	DecodeAbi("", "", "")
	
	//延时处理;
	for {
		time.Sleep(1 * time.Minute)
	}
	
}*/
