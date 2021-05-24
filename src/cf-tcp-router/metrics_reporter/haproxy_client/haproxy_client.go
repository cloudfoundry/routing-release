package haproxy_client

import (
	"bytes"
	"encoding/csv"
	"io"
	"net"
	"strconv"
	"time"

	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter -o fakes/fake_haproxy_client.go . HaproxyClient
type HaproxyClient interface {
	GetStats() HaproxyStats
}

type HaproxyStatsClient struct {
	haproxyUnixSocket string
	timeout           time.Duration
	logger            lager.Logger
}

type HaproxyStats []HaproxyStat

type HaproxyStat struct {
	ProxyName            string `csv:"pxname"`
	CurrentQueued        uint64 `csv:"qcur"`
	CurrentSessions      uint64 `csv:"scur"`
	ErrorConnecting      uint64 `csv:"econ"`
	AverageQueueTimeMs   uint64 `csv:"qtime"`
	AverageConnectTimeMs uint64 `csv:"ctime"`
	AverageSessionTimeMs uint64 `csv:"ttime"`
}

func NewClient(logger lager.Logger, haproxyUnixSocket string, timeout time.Duration) *HaproxyStatsClient {
	return &HaproxyStatsClient{
		haproxyUnixSocket: haproxyUnixSocket,
		timeout:           timeout,
		logger:            logger,
	}
}

func (r *HaproxyStatsClient) GetStats() HaproxyStats {
	logger := r.logger.Session("get-stats")
	logger.Debug("start")
	defer logger.Debug("completed")

	buff := make([]byte, 1024)
	b := make([]byte, 0)
	buffer := bytes.NewBuffer(b)

	stats := HaproxyStats{}

	conn, err := net.DialTimeout("unix", r.haproxyUnixSocket, r.timeout)
	if err != nil {
		logger.Error("error-connecting-to-haproxy-stats", err)
		return stats
	}
	defer conn.Close()
	logger.Debug("connection-successful")

	_, err = conn.Write([]byte("show stat\n"))
	if err != nil {
		logger.Error("error-sending-haproxy-stats-command", err)
		return stats
	}
	logger.Debug("sent-stats-command")

	for {
		cnt, err := conn.Read(buff[:])
		if err != nil {
			if err == io.EOF {
				break
			} else {
				logger.Error("error-reading-haproxy-stats", err)
				return stats
			}
		}
		buffer.Write(buff[:cnt])
	}
	logger.Debug("num-bytes-read", lager.Data{"count": buffer.Len()})
	if buffer.Len() > 0 {
		stats = readCsv(logger, buffer.Bytes())
	}
	return stats
}

func readCsv(logger lager.Logger, buffer []byte) HaproxyStats {
	stats := HaproxyStats{}

	bReader := bytes.NewReader(buffer)
	csvReader := csv.NewReader(bReader)

	lines, err := csvReader.ReadAll()
	if err != nil {
		logger.Error("error-reading-csv-stats", err)
		return stats
	}

	for i, line := range lines {
		if i == 0 {
			// skip header line
			continue
		}
		stats = append(stats, csvToHaproxyStat(line))
	}
	return stats
}

func csvToHaproxyStat(row []string) HaproxyStat {
	return HaproxyStat{
		ProxyName:            row[0],
		CurrentQueued:        convertToInt(row[2]),
		CurrentSessions:      convertToInt(row[4]),
		ErrorConnecting:      convertToInt(row[13]),
		AverageQueueTimeMs:   convertToInt(row[58]),
		AverageConnectTimeMs: convertToInt(row[59]),
		AverageSessionTimeMs: convertToInt(row[61]),
	}
}

func convertToInt(s string) uint64 {
	i, _ := strconv.ParseUint(s, 10, 64)
	return i
}
