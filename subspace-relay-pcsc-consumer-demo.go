package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/nvx/go-apdu"
	"github.com/nvx/go-rfid"
	subspacerelay "github.com/nvx/go-subspace-relay"
	"github.com/nvx/go-subspace-relay-logger"
	subspacerelaypb "github.com/nvx/subspace-relay"
	"log/slog"
	"os"
	"strings"
)

// This can be set at build time using the following go build command:
// go build -ldflags="-X 'main.defaultBrokerURL=mqtts://user:pass@example.com:1234'"
var defaultBrokerURL string

func main() {
	ctx := context.Background()
	var (
		relayID    = flag.String("relay-id", "", "Subspace Relay ID to connect to")
		brokerFlag = flag.String("broker-url", "", "MQTT Broker URL")
		capdu      = flag.String("capdu", "FFCA000000", "cAPDU to run")
	)
	flag.Parse()

	srlog.InitLogger("subspace-relay-pcsc-consumer-demo")

	brokerURL := subspacerelay.NotZero(*brokerFlag, os.Getenv("BROKER_URL"), defaultBrokerURL)
	if brokerURL == "" {
		slog.ErrorContext(ctx, "No broker URI specified, either specify as a flag or set the BROKER_URI environment variable")
		flag.Usage()
		os.Exit(2)
	}

	capduBytes, err := hex.DecodeString(*capdu)
	if err != nil {
		slog.ErrorContext(ctx, "Error decoding cAPDU hex bytes")
		flag.Usage()
		os.Exit(2)
	}

	if *relayID == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Subspace Relay Client ID (PCSC HF): ")
		var err error
		*relayID, err = reader.ReadString('\n')
		if err != nil {
			return
		}
		*relayID = strings.TrimSpace(*relayID)
	}

	// The connection type can be omitted to allow connecting to any PCSC reader (direct or regular), or changed to
	// ConnectionType_CONNECTION_TYPE_PCSC_DIRECT to instead ensure the remote end is a direct connection
	reader, err := subspacerelay.NewPCSC(ctx, brokerURL, *relayID, subspacerelaypb.ConnectionType_CONNECTION_TYPE_PCSC)
	if err != nil {
		slog.ErrorContext(ctx, "Error opening Subspace Relay PCSC connection", rfid.ErrorAttrs(err))
		os.Exit(1)
	}

	err = run(ctx, reader, capduBytes)
	if err != nil {
		slog.ErrorContext(ctx, "Error", rfid.ErrorAttrs(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, reader *subspacerelay.PCSC, capduBytes []byte) (err error) {
	defer rfid.DeferWrap(ctx, &err)

	// In your own code you might want to construct the cAPDU with github.com/nvx/go-apdu
	//capduBytes, err := apdu.Capdu{
	//	CLA: 0xFF,
	//	INS: 0xCA, // GET DATA
	//	P1:  0x00, // UID
	//	P2:  0x00,
	//	Ne:  apdu.MaxLenResponseDataStandard,
	//}.Bytes()
	//if err != nil {
	//	return
	//}

	rapduBytes, err := reader.Exchange(ctx, capduBytes)
	if err != nil {
		return
	}

	rapdu, err := apdu.ParseRapdu(rapduBytes)
	if err != nil {
		return
	}

	slog.InfoContext(ctx, "Got response", "rapdu", rapdu)

	return nil
}
