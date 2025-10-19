package main

import (
	"bufio"
	"context"
	"crypto/ecdh"
	"crypto/rand"
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
		err          error
		relayID      = flag.String("relay-id", "", "Subspace Relay ID to connect to")
		useDiscovery = flag.Bool("discovery", false, "Use discovery")
		discoveryKey = flag.String("discovery.secure", "*", "When discovery is enabled use this private key, if * a random key will be generated and printed. If empty will use plaintext discovery")
		brokerFlag   = flag.String("broker-url", "", "MQTT Broker URL")
		capdu        = flag.String("capdu", "FFCA000000", "cAPDU to run")
	)
	flag.Parse()

	srlog.InitLogger("subspace-relay-pcsc-controller-demo")

	brokerURL := subspacerelay.NotZero(*brokerFlag, os.Getenv("BROKER_URL"), defaultBrokerURL)
	if brokerURL == "" {
		slog.ErrorContext(ctx, "No broker URI specified, either specify as a flag or set the BROKER_URI environment variable")
		flag.Usage()
		os.Exit(2)
	}

	capduBytes, err := hex.DecodeString(*capdu)
	if err != nil {
		slog.ErrorContext(ctx, "Error decoding cAPDU hex bytes", rfid.ErrorAttrs(err))
		flag.Usage()
		os.Exit(2)
	}

	if *useDiscovery {
		if *relayID != "" {
			slog.ErrorContext(ctx, "Cannot specify relay id with discovery")
			flag.Usage()
			os.Exit(2)
		}

		var key *ecdh.PrivateKey
		if *discoveryKey == "*" {
			key, err = ecdh.X25519().GenerateKey(rand.Reader)
			if err != nil {
				slog.ErrorContext(ctx, "Error generating key", rfid.ErrorAttrs(err))
				os.Exit(1)
			}
			slog.InfoContext(ctx, "Generated key", rfid.LogHex("public_key", key.PublicKey().Bytes()), rfid.LogHex("private_key", key.Bytes()))
		} else if *discoveryKey != "" {
			var keyBytes []byte
			keyBytes, err = hex.DecodeString(*discoveryKey)
			if err != nil {
				slog.ErrorContext(ctx, "Error decoding discovery key hex bytes", rfid.ErrorAttrs(err))
				flag.Usage()
				os.Exit(2)
			}

			key, err = ecdh.X25519().NewPrivateKey(keyBytes)
			if err != nil {
				slog.ErrorContext(ctx, "Error parsing provided discovery private key", rfid.ErrorAttrs(err))
				flag.Usage()
				os.Exit(2)
			}

			slog.InfoContext(ctx, "Using discovery key", rfid.LogHex("public_key", key.PublicKey().Bytes()))
		}

		var discovery *subspacerelay.Discovery
		discovery, err = subspacerelay.NewDiscovery(ctx, brokerURL,
			subspacerelay.WithDiscoveryPayloadType(subspacerelaypb.PayloadType_PAYLOAD_TYPE_PCSC_READER),
			subspacerelay.WithDiscoveryPrivateKey(key),
		)
		if err != nil {
			slog.ErrorContext(ctx, "Error during discovery", rfid.ErrorAttrs(err))
			os.Exit(1)
		}

		slog.InfoContext(ctx, "Waiting for discovery")
		relayDiscovery := <-discovery.Chan()

		// some implementations may opt to keep discovery running and continually service relays as they come online
		// otherwise it is important to call Close to free resources
		err = discovery.Close()
		if err != nil {
			slog.ErrorContext(ctx, "Error shutting down discovery", rfid.ErrorAttrs(err))
			os.Exit(1)
		}

		slog.InfoContext(ctx, "Discovered relay", slog.String("relay_id", relayDiscovery.RelayId))
		*relayID = relayDiscovery.RelayId
	}

	if *relayID == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Subspace Relay ID: ")
		*relayID, err = reader.ReadString('\n')
		if err != nil {
			slog.ErrorContext(ctx, "Error reading Relay ID from stdin", rfid.ErrorAttrs(err))
			os.Exit(1)
		}
		*relayID = strings.TrimSpace(*relayID)
	}

	// The connection type can be omitted to allow connecting to any PCSC reader (direct or regular), or changed to
	// ConnectionType_CONNECTION_TYPE_PCSC_DIRECT to instead ensure the remote end is a direct connection
	reader, err := subspacerelay.NewPCSC(ctx, brokerURL, *relayID,
		subspacerelaypb.ConnectionType_CONNECTION_TYPE_PCSC,
		subspacerelaypb.ConnectionType_CONNECTION_TYPE_NFC,
	)
	if err != nil {
		slog.ErrorContext(ctx, "Error opening Subspace Relay PCSC connection", rfid.ErrorAttrs(err))
		os.Exit(1)
	}
	defer reader.Close()

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
