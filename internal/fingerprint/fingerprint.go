package fingerprint

import (
	"fmt"
	"gonum.org/v1/gonum/dsp/fourier"
	"math"
	"sync"
	"runtime"
)

const (
	fftWindowSize=4096
	fftOverLap=2048
	peakNeighborhood=10
	targetZoneHeight=90
	targetZoneWidth=45
)

func GenerateSpectogram(monoSamples []float64,sampleRate int) ([][]float64,error){
	fmt.Println("fingerprint: Generating fingerprints...")
	// Debug: check input sample range
		if len(monoSamples) > 0 {
			min, max := monoSamples[0], monoSamples[0]
			for _, s := range monoSamples {
				if s < min {
					min = s
				}
				if s > max {
					max = s
				}
			}
			fmt.Printf("fingerprint: Input sample range: [%.6f, %.6f]\n", min, max)
	}
	var spectrogram [][]float64
	// Create Hann window manually: w[k] = 0.5*(1 - cos(2*π*k/(N-1)))
		hann := make([]float64, fftWindowSize)
		if fftWindowSize > 1 {
			for i := 0; i < fftWindowSize; i++ {
				hann[i] = 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(fftWindowSize-1)))
			}
	} else {
		hann[0] = 1.0
	}
	// Debug: check window values
		hannMin, hannMax := hann[0], hann[0]
		for _, v := range hann {
			if v < hannMin {
				hannMin = v
			}
			if v > hannMax {
				hannMax = v
			}
	}
	fmt.Printf("fingerprint: Hann window range: [%.6f, %.6f]\n", hannMin, hannMax)
	fmt.Printf("fingerprint: Hann window first 5 values: [%.6f, %.6f, %.6f, %.6f, %.6f]\n", 
		hann[0], hann[1], hann[2], hann[3], hann[4])
	fft:=fourier.NewFFT(fftWindowSize)
	size:=len(monoSamples)
	// fftWindowSize:=512
	// fftOverLap:=256
	segmentCount := 0
	for i:=0;i<=size-fftWindowSize;i+=fftWindowSize-fftOverLap{
		chunk:=make([]float64,fftWindowSize)
		copy(chunk,monoSamples[i:i+fftWindowSize])
		// Debug first chunk
		if segmentCount == 0 {
			chunkMin, chunkMax := chunk[0], chunk[0]
			for _, v := range chunk {
				if v < chunkMin {
					chunkMin = v
				}
				if v > chunkMax {
					chunkMax = v
				}
			}
			fmt.Printf("fingerprint: First chunk range (before window): [%.6f, %.6f]\n", chunkMin, chunkMax)
		}
		for j:=0;j<fftWindowSize;j++{
			chunk[j]*=hann[j]
		}
		// Debug first chunk after windowing
		if segmentCount == 0 {
			chunkMin, chunkMax := chunk[0], chunk[0]
			for _, v := range chunk {
				if v < chunkMin {
					chunkMin = v
				}
				if v > chunkMax {
					chunkMax = v
				}
			}
			fmt.Printf("fingerprint: First chunk range (after window): [%.6f, %.6f]\n", chunkMin, chunkMax)
		}
		coeff:=fft.Coefficients(nil,chunk)
		// Debug first FFT coefficients
		if segmentCount == 0 && len(coeff) > 0 {
			fmt.Printf("fingerprint: First FFT coeff[0]: real=%.6f, imag=%.6f\n", real(coeff[0]), imag(coeff[0]))
			if len(coeff) > 1 {
				fmt.Printf("fingerprint: First FFT coeff[1]: real=%.6f, imag=%.6f\n", real(coeff[1]), imag(coeff[1]))
			}
		}
		magnitudes:=make([]float64,len(coeff))
		for j,c:=range coeff{
			magnitudes[j]=math.Sqrt(real(c)*real(c)+imag(c)*imag(c))
		}
		// Debug first magnitudes
		if segmentCount == 0 && len(magnitudes) > 0 {
			magMin, magMax := magnitudes[0], magnitudes[0]
			for _, v := range magnitudes {
				if v < magMin {
					magMin = v
				}
				if v > magMax {
					magMax = v
				}
			}
			fmt.Printf("fingerprint: First segment magnitude range: [%.6f, %.6f]\n", magMin, magMax)
		}
		spectrogram=append(spectrogram,magnitudes)
		segmentCount++
	}
	// Debug: check max magnitude in spectrogram
	maxMag := 0.0
	for _, row := range spectrogram {
		for _, val := range row {
			if val > maxMag {
				maxMag = val
			}
		}
	}
	fmt.Printf("fingerprint: Max magnitude in spectrogram: %f\n", maxMag)
	return spectrogram,nil
}

type Peak struct{
	Time int
	Freq int
}

func ExtractPeaks(spectrogram [][]float64,sampleRate int) ([]Peak,error){
	var peaks []Peak
	// Debug: count points above threshold
	aboveThreshold := 0
	for r:=0;r<len(spectrogram);r++{
		for c:=0;c<len(spectrogram[r]);c++{
			if spectrogram[r][c] > 0.0001 {
				aboveThreshold++
			}
		}
	}
	fmt.Printf("fingerprint: Points above threshold (0.1): %d\n", aboveThreshold)

	for r:=0;r<len(spectrogram);r++{
		for c:=0;c<len(spectrogram[r]);c++{
		// Only consider peaks above a minimum magnitude threshold
		if spectrogram[r][c] < 0.00000000000000001 {
			continue
		}
		//now to create the box
		maxVal:=spectrogram[r][c]
		for nr:=r-peakNeighborhood;nr<=r+peakNeighborhood;nr++{
			for nc:=c-peakNeighborhood;nc<=c+peakNeighborhood;nc++{
				if nr<0 || nr>=len(spectrogram) || nc<0 || nc>=len(spectrogram[nr]){
					continue
				}
				if spectrogram[nr][nc]>maxVal{
					maxVal=spectrogram[nr][nc]
				}
			}
		}
		if spectrogram[r][c]==maxVal{
			peaks=append(peaks,Peak{
				Time: r,
				Freq: c,
			})
		}
		}
	}
	return peaks,nil
}

type workerResult struct{
	hash uint32
	time float64
}

func GenerateHashes(peaks []Peak, sampleRate int) (map[uint32]float64, error) {
	numWorkers := runtime.NumCPU()
	jobsChan := make(chan int, len(peaks))
	resultsChan := make(chan workerResult, len(peaks))

	var wg sync.WaitGroup
	worker := func(workerID int) {
		defer wg.Done()
		for anchorIndex := range jobsChan {
			anchor := peaks[anchorIndex]
			for j := anchorIndex + 1; j < len(peaks) && (peaks[j].Time-anchor.Time) <= targetZoneHeight; j++ {
				target := peaks[j]
				if math.Abs(float64(target.Freq-anchor.Freq)) <= float64(targetZoneWidth) {
					timeDelta := target.Time - anchor.Time
					hash := (uint32(anchor.Freq) << 22) | (uint32(target.Freq) << 12) | (uint32(timeDelta))
					anchorTime := float64(anchor.Time*(fftWindowSize-fftOverLap)) / float64(sampleRate)
					fmt.Printf("Generated hash: %d at time: %f\n",hash,anchorTime)
					resultsChan <- workerResult{
						hash: hash,
						time: anchorTime,
					}
				}
			}
		}
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(i)
	}

	for i := range peaks {
		jobsChan <- i
	}
	close(jobsChan)

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	finalHashes := make(map[uint32]float64)
	for result := range resultsChan {
		finalHashes[result.hash] = result.time
	}

	return finalHashes, nil
}

