// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package gen

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

func M(a ...interface{}) []interface{} {
	return a
}

func TestBinDataFileInfo(t *testing.T) {
	_time := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	bindataFileInfo := bindataFileInfo{
		name:    "bindata.go",
		size:    0,
		mode:    os.FileMode(0),
		modTime: _time,
	}
	assert.Equal(t, "bindata.go", bindataFileInfo.Name())
	assert.Equal(t, int64(0), bindataFileInfo.Size())
	assert.Equal(t, os.FileMode(0), bindataFileInfo.Mode())
	assert.Equal(t, _time, bindataFileInfo.ModTime())
	assert.Equal(t, false, bindataFileInfo.IsDir())
	assert.Nil(t, bindataFileInfo.Sys())
}

func TestBinDataRead(t *testing.T) {
	bytes, _ := bindataRead(_compiledAuthorityAbi, "compiled/Authority.abi")
	assetBytes, _ := compiledAuthorityAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledAuthorityBinRuntime, "compiled/Authority.bin-runtime")
	assetBytes, _ = compiledAuthorityBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledAuthoritynativeAbi, "compiled/AuthorityNative.abi")
	assetBytes, _ = compiledAuthoritynativeAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledAuthoritynativeBinRuntime, "compiled/AuthorityNative.bin-runtime")
	assetBytes, _ = compiledAuthoritynativeBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledEnergyAbi, "compiled/Energy.abi")
	assetBytes, _ = compiledEnergyAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledEnergyBinRuntime, "compiled/Energy.bin-runtime")
	assetBytes, _ = compiledEnergyBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledEnergynativeAbi, "compiled/EnergyNative.abi")
	assetBytes, _ = compiledEnergynativeAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledEnergynativeBinRuntime, "compiled/EnergyNative.bin-runtime")
	assetBytes, _ = compiledEnergynativeBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExecutorAbi, "compiled/Executor.abi")
	assetBytes, _ = compiledExecutorAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExecutorBinRuntime, "compiled/Executor.bin-runtime")
	assetBytes, _ = compiledExecutorBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExtensionAbi, "compiled/Extension.abi")
	assetBytes, _ = compiledExtensionAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExtensionBinRuntime, "compiled/Extension.bin-runtime")
	assetBytes, _ = compiledExtensionBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExtensionnativeAbi, "compiled/ExtensionNative.abi")
	assetBytes, _ = compiledExtensionnativeAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExtensionnativeBinRuntime, "compiled/ExtensionNative.bin-runtime")
	assetBytes, _ = compiledExtensionnativeBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExtensionv2Abi, "compiled/ExtensionV2.abi")
	assetBytes, _ = compiledExtensionv2Abi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExtensionv2BinRuntime, "compiled/ExtensionV2.bin-runtime")
	assetBytes, _ = compiledExtensionv2BinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExtensionv2nativeAbi, "compiled/ExtensionV2Native.abi")
	assetBytes, _ = compiledExtensionv2nativeAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledExtensionv2nativeBinRuntime, "compiled/ExtensionV2Native.bin-runtime")
	assetBytes, _ = compiledExtensionv2nativeBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledMeasureAbi, "compiled/Measure.abi")
	assetBytes, _ = compiledMeasureAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledMeasureBinRuntime, "compiled/Measure.bin-runtime")
	assetBytes, _ = compiledMeasureBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledParamsAbi, "compiled/Params.abi")
	assetBytes, _ = compiledParamsAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledParamsBinRuntime, "compiled/Params.bin-runtime")
	assetBytes, _ = compiledParamsBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledParamsnativeAbi, "compiled/ParamsNative.abi")
	assetBytes, _ = compiledParamsnativeAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledParamsnativeBinRuntime, "compiled/ParamsNative.bin-runtime")
	assetBytes, _ = compiledParamsnativeBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledPrototypeAbi, "compiled/Prototype.abi")
	assetBytes, _ = compiledPrototypeAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledPrototypeBinRuntime, "compiled/Prototype.bin-runtime")
	assetBytes, _ = compiledPrototypeBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledPrototypeeventAbi, "compiled/PrototypeEvent.abi")
	assetBytes, _ = compiledPrototypeeventAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledPrototypeeventBinRuntime, "compiled/PrototypeEvent.bin-runtime")
	assetBytes, _ = compiledPrototypeeventBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledPrototypenativeAbi, "compiled/PrototypeNative.abi")
	assetBytes, _ = compiledPrototypenativeAbi()
	assert.Equal(t, bytes, assetBytes.bytes)

	bytes, _ = bindataRead(_compiledPrototypenativeBinRuntime, "compiled/PrototypeNative.bin-runtime")
	assetBytes, _ = compiledPrototypenativeBinRuntime()
	assert.Equal(t, bytes, assetBytes.bytes)
}

func TestAsset(t *testing.T) {
	bytes, _ := Asset("compiled/Authority.abi")
	compiledAuthorityAbiBytes, _ := compiledAuthorityAbi()
	assert.Equal(t, compiledAuthorityAbiBytes.bytes, bytes)

	bytes, _ = Asset("Invalid File")
	assert.Nil(t, bytes)
}

func TestAssetInfo(t *testing.T) {
	fileInfo, _ := AssetInfo("compiled/Authority.abi")
	compiledAuthorityAbiBytes, _ := compiledAuthorityAbi()
	assert.Equal(t, compiledAuthorityAbiBytes.info, fileInfo)

	fileInfo, _ = AssetInfo("Invalid File")
	assert.Nil(t, fileInfo)
}

func TestAssetNames(t *testing.T) {
	assert.NotEmpty(t, AssetNames())
}

func TestAssetDir(t *testing.T) {
	dir, _ := AssetDir("compiled")
	assert.NotEmpty(t, dir)
}

func TestRestoreAsset(t *testing.T) {
	assert.Nil(t, RestoreAsset("compiled", "compiled/Authority.abi"))
	os.RemoveAll("compiled")
}

func TestRestoreAssets(t *testing.T) {
	assert.Nil(t, RestoreAssets("compiled", "compiled/Authority.abi"))
	os.RemoveAll("compiled")
}
