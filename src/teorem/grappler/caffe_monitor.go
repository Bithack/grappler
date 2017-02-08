package main

import (
	"bufio"
	"net/http"
	"os"
	"strconv"
	"strings"

	"regexp"

	"github.com/gonum/matrix/mat64"
	chart "github.com/wcharczuk/go-chart"
)

func caffeMonitor(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "text/html")
	res.Write([]byte("<html><head><title>Grappler</title></head><body>"))
	res.Write([]byte("<div style=\"font-family:monospace\">" + strings.Replace(strings.Replace(grapplerLogo, "\n", "<br>", -1), " ", "&nbsp;", -1) + "</div><br>"))
	res.Write([]byte("<image src=\"/graph\"><br>"))
	res.Write([]byte("</body></html>"))
}

func caffeGraph(res http.ResponseWriter, req *http.Request) {
	train, test, err := parseLogFile("/tmp/caffe.INFO")
	if err != nil {
		res.Write([]byte("Error parsing logfile<br>"))
		return
	}
    var testSeries, trainSeries chart.ContinuousSeries
    var series []chart.Series
    if train != nil {
	    t2 := mat64.DenseCopyOf(train.T())
        trainSeries = chart.ContinuousSeries{
            Name:    "Train loss",
            YAxis:   chart.YAxisPrimary,
            YValues: t2.RawRowView(1),
            XValues: t2.RawRowView(0),
        }
        series = append(series, trainSeries)
        matrixes["trainLog"] = train
    }
    if test != nil {
        t3 := mat64.DenseCopyOf(test.T())
        testSeries = chart.ContinuousSeries{
            Name: "Test loss",
            Style: chart.Style{
                Show:        true,
                StrokeWidth: 2.0,
                StrokeColor: chart.ColorRed,
            },
            YAxis:   chart.YAxisPrimary,
            YValues: t3.RawRowView(1),
            XValues: t3.RawRowView(0),
        }
        series = append(series, testSeries)
        matrixes["testLog"] = test
    }

	/*minSeries := &chart.MinSeries{
		Style: chart.Style{
			Show:            true,
			StrokeColor:     chart.ColorAlternateGray,
			StrokeDashArray: []float64{5.0, 5.0},
		},
		InnerSeries: trainSeries,
	}
	maxSeries := &chart.MaxSeries{
		Style: chart.Style{
			Show:            true,
			StrokeColor:     chart.ColorAlternateGray,
			StrokeDashArray: []float64{5.0, 5.0},
		},
		InnerSeries: trainSeries,
	}*/

	graph := chart.Chart{
		Width:  1200,
		Height: 600,
		YAxis: chart.YAxis{
			Name: "Loss",
            NameStyle: chart.Style {
                Show: true,
            },
            Zero: chart.GridLine {
                Value: 0,
                Style: chart.Style {
                    Show: true,
                    StrokeColor: chart.ColorBlack,
                },
            },
            GridLines: []chart.GridLine{
		        {Value: 0},{Value: 0.001},{Value: 0.002},{Value: 0.003},{Value: 0.004},
            },
            GridMajorStyle: chart.Style{
				Show:        true,
				StrokeColor: chart.ColorAlternateGray,
				StrokeWidth: 0.5,
			},
            ValueFormatter: func(v interface{}) string {
                return chart.FloatValueFormatterWithFormat(v, "%0.4f")
            },
			//Ticks: chart.Sequence.Float64(0, 1, 0.1),
			Style: chart.Style{
				Show: true, //enables / displays the y-axis
			},
		},
		XAxis: chart.XAxis{
			Name: "Iteration",
			Style: chart.Style{
				Show: true, //enables / displays the x-axis
			},
		},
		Series: series,
	}
	res.Header().Set("Content-Type", "image/svg+xml")
	graph.Render(chart.SVG, res)
}

type logRow struct {
	iteration float64
	loss      float64
}

func parseLogFile(path string) (train *mat64.Dense, test *mat64.Dense, err error) {
	f, _ := os.Open(path)
	scanner := bufio.NewScanner(f)
	var currentIteration float64
	var trainResults []logRow
	var testResults []logRow
	for scanner.Scan() {
		text := scanner.Text()
		iterationRE := regexp.MustCompile("Iteration (\\d+)")
		t := iterationRE.FindStringSubmatch(text)
		if len(t) > 0 {
			currentIteration, _ = strconv.ParseFloat(t[1], 64)
		}
		trainRE := regexp.MustCompile("Train net output #(\\d+): (\\S+) = ([\\.\\deE+-]+)")
		t = trainRE.FindStringSubmatch(text)
		if len(t) > 0 {
			loss, _ := strconv.ParseFloat(t[3], 64)
			trainResults = append(trainResults, logRow{iteration: currentIteration, loss: loss})
		}
		testRE := regexp.MustCompile("Test net output #(\\d+): (\\S+) = ([\\.\\deE+-]+)")
		t = testRE.FindStringSubmatch(text)
		if len(t) > 0 {
			loss, _ := strconv.ParseFloat(t[3], 64)
			testResults = append(testResults, logRow{iteration: currentIteration, loss: loss})
		}
	}
	f.Close()
    if len(trainResults) > 0 {
        train = mat64.NewDense(len(trainResults), 2, nil)
        for i, v := range trainResults {
            train.Set(i, 0, v.iteration)
            train.Set(i, 1, v.loss)
        }
    }	
    if len(testResults) > 0 {
        test = mat64.NewDense(len(testResults), 2, nil)
        for i, v := range testResults {
            test.Set(i, 0, v.iteration)
            test.Set(i, 1, v.loss)
        }
    }
	return
}
