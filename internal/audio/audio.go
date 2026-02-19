package audio

import (
	"fmt"
	"os"
	"github.com/go-audio/wav"
)

func LoadWav(path string) ([]float64,int,error) {
	fmt.Println("audio: Loading WAV file...")
	file,err:=os.Open(path)
	if err!=nil{
		return nil,0,err
	}
	defer file.Close()

	decoder:=wav.NewDecoder(file)
	if !decoder.IsValidFile() {
		return nil, 0, fmt.Errorf("invalid WAV file")
	}

	buf,err:=decoder.FullPCMBuffer()
	if err!=nil{
		return nil,0,err
	}

	sampleRate:=int(decoder.SampleRate)
	format := buf.Format
	numChannels:=int(format.NumChannels)
	maxAbsValue:=0
	for _, sample:=range buf.Data {
		abs:=sample
		if abs<0 {
			abs=-abs
		}
		if abs>maxAbsValue {
			maxAbsValue=abs
		}
	}
	// Infer bit depth from maximum value and calculate normalization factor
	// For 16-bit: max value is 32767, so divide by 32768.0
	// For 24-bit: max value is 8388607, so divide by 8388608.0
	// For 32-bit: max value is 2147483647, so divide by 2147483648.0
	var maxValue float64
	var bitDepth int
	if maxAbsValue <= 127 {
		bitDepth = 8
		maxValue = 128.0
	} else if maxAbsValue <= 32767 {
		bitDepth = 16
		maxValue = 32768.0
	} else if maxAbsValue <= 8388607 {
		bitDepth = 24
		maxValue = 8388608.0
	} else {
		bitDepth = 32
		maxValue = 2147483648.0
	}

	fmt.Printf("audio: Format - %d channels, %d-bit (inferred), %d Hz\n", numChannels, bitDepth, sampleRate)
	samples:=make([]float64,len(buf.Data))
	for i,sample:=range buf.Data {
		// Normalize to [-1.0, 1.0] range
		samples[i]=float64(sample)/maxValue
	}
	// If stereo, convert to mono
	if numChannels == 2 {
		samples = ToMono(samples)
	}

	return samples,sampleRate,nil
}

func ToMono(stereoSamples []float64) []float64 {
	monoSamples:=make([]float64,len(stereoSamples)/2)
	for i:=0;i<len(monoSamples);i++{
		left:=stereoSamples[i*2]
		right:=stereoSamples[i*2+1]
		monoSamples[i]=(left+right)/2.0
	}
	return monoSamples
}
