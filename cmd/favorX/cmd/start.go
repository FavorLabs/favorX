package cmd

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	favor "github.com/FavorLabs/favorX"
	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/crypto"
	"github.com/FavorLabs/favorX/pkg/keystore/p2pkey"
	"github.com/FavorLabs/favorX/pkg/keystore/subkey"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/node"
	"github.com/FavorLabs/favorX/pkg/resolver/multiresolver"
	"github.com/FavorLabs/favorX/pkg/storagefiles"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

const (
	serviceName = "favorXSvc"
)

//go:embed welcome-message.txt
var welcomeMessage string

func (c *command) initStartCmd() (err error) {

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a favorX node",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			v := strings.ToLower(c.config.GetString(optionNameVerbosity))
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %v", err)
			}

			go startTimeBomb(logger)

			isWinService, err := isWindowsService()
			if err != nil {
				return fmt.Errorf("failed to determine if we are running in service: %w", err)
			}

			if isWinService {
				var err error
				logger, err = createWindowsEventLogger(serviceName, logger)
				if err != nil {
					return fmt.Errorf("failed to create windows logger %w", err)
				}
			}

			// If the resolver is specified, resolve all connection strings
			// and fail on any errors.
			var resolverCfgs []multiresolver.ConnectionConfig
			resolverEndpoints := c.config.GetStringSlice(optionNameResolverEndpoints)
			if len(resolverEndpoints) > 0 {
				resolverCfgs, err = multiresolver.ParseConnectionStrings(resolverEndpoints)
				if err != nil {
					return err
				}
			}

			fmt.Print(welcomeMessage)

			debugAPIAddr := c.config.GetString(optionNameDebugAPIAddr)
			if !c.config.GetBool(optionNameDebugAPIEnable) {
				debugAPIAddr = ""
			}

			signerCfg, err := c.configureSigner(cmd, logger)
			if err != nil {
				return err
			}

			logger.Infof("version: %v", favor.Version)

			bootNode := c.config.GetBool(optionNameBootnodeMode)
			fullNode := c.config.GetBool(optionNameFullNode)
			mode := address.NewModel()
			if bootNode {
				mode.SetMode(address.BootNode)
				mode.SetMode(address.FullNode)
				logger.Info("Start node mode boot.")
			} else if fullNode {
				mode.SetMode(address.FullNode)
				logger.Info("Start node mode full.")
			} else {
				logger.Info("Start node mode light.")
			}

			b, err := node.NewNode(mode, c.config.GetString(optionNameP2PAddr), c.config.GetUint64(optionNameNetworkID), logger, signerCfg, node.Options{
				DataDir:                c.config.GetString(optionNameDataDir),
				CacheCapacity:          c.config.GetUint64(optionNameCacheCapacity),
				DBDriver:               c.config.GetString(optionDatabaseDriver),
				DBPath:                 c.config.GetString(optionDatabasePath),
				HTTPAddr:               c.config.GetString(optionNameHTTPAddr),
				WSAddr:                 c.config.GetString(optionNameWebsocketAddr),
				APIAddr:                c.config.GetString(optionNameAPIAddr),
				DebugAPIAddr:           debugAPIAddr,
				ApiBufferSizeMul:       c.config.GetInt(optionNameApiFileBufferMultiple),
				NATAddr:                c.config.GetString(optionNameNATAddr),
				EnableWS:               c.config.GetBool(optionNameP2PWSEnable),
				EnableQUIC:             c.config.GetBool(optionNameP2PQUICEnable),
				WelcomeMessage:         c.config.GetString(optionWelcomeMessage),
				Bootnodes:              c.config.GetStringSlice(optionNameBootnodes),
				ChainEndpoint:          c.config.GetString(optionNameChainEndpoint),
				OracleContractAddress:  c.config.GetString(optionNameOracleContractAddr),
				CORSAllowedOrigins:     c.config.GetStringSlice(optionCORSAllowedOrigins),
				Standalone:             c.config.GetBool(optionNameStandalone),
				IsDev:                  c.config.GetBool(optionNameDevMode),
				TracingEnabled:         c.config.GetBool(optionNameTracingEnabled),
				TracingEndpoint:        c.config.GetString(optionNameTracingEndpoint),
				TracingServiceName:     c.config.GetString(optionNameTracingServiceName),
				Logger:                 logger,
				ResolverConnectionCfgs: resolverCfgs,
				GatewayMode:            c.config.GetBool(optionNameGatewayMode),
				TrafficEnable:          c.config.GetBool(optionNameTrafficEnable),
				TrafficContractAddr:    c.config.GetString(optionNameTrafficContractAddr),
				KadBinMaxPeers:         c.config.GetInt(optionNameBinMaxPeers),
				LightNodeMaxPeers:      c.config.GetInt(optionNameLightMaxPeers),
				AllowPrivateCIDRs:      c.config.GetBool(optionNameAllowPrivateCIDRs),
				Restricted:             c.config.GetBool(optionNameRestrictedAPI),
				TokenEncryptionKey:     c.config.GetString(optionNameTokenEncryptionKey),
				AdminPasswordHash:      c.config.GetString(optionNameAdminPasswordHash),
				RouteAlpha:             c.config.GetInt32(optionNameRouteAlpha),
				Groups:                 c.config.Get(optionNameGroups),
				EnableApiTLS:           c.config.GetBool(optionNameEnableApiTls),
				TlsCrtFile:             c.config.GetString(optionNameTlsCRT),
				TlsKeyFile:             c.config.GetString(optionNameTlsKey),
				StorageFilesEnable:     c.config.GetBool(optionNameStorageEnable),
				StorageFilesConfig: storagefiles.Config{
					Overlay:     signerCfg.Overlay,
					Capacity:    c.config.GetUint64(optionNameStorageCapacity),
					CacheDir:    c.config.GetString(optionNameStorageCacheDir),
					DelFileTime: c.config.GetInt64(optionNameStorageDelFileTime),
					BlockSize:   c.config.GetInt(optionNameStorageBlockSize),
					RetryNumber: c.config.GetInt(optionNameStorageRetryNumber),
					Redundant:   c.config.GetUint64(optionNameStorageRedundantDisk),
				},
			})
			if err != nil {
				return err
			}

			// Wait for termination or interrupt signals.
			// We want to clean up things at the end.
			interruptChannel := make(chan os.Signal, 1)
			signal.Notify(interruptChannel, syscall.SIGINT, syscall.SIGTERM)

			p := &program{
				start: func() {
					// Block main goroutine until it is interrupted
					sig := <-interruptChannel

					logger.Debugf("received signal: %v", sig)
					logger.Info("shutting down")
				},
				stop: func() {
					// Shutdown
					done := make(chan struct{})
					go func() {
						defer close(done)

						ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
						defer cancel()

						if err := b.Shutdown(ctx); err != nil {
							logger.Errorf("shutdown: %v", err)
						}
					}()

					// If shutdown function is blocking too long,
					// allow process termination by receiving another signal.
					select {
					case sig := <-interruptChannel:
						logger.Debugf("received signal: %v", sig)
					case <-done:
					}
				},
			}

			if isWinService {
				s, err := service.New(p, &service.Config{
					Name:        serviceName,
					DisplayName: "favorX",
					Description: "favorX client.",
				})
				if err != nil {
					return err
				}

				if err = s.Run(); err != nil {
					return err
				}
			} else {
				// start blocks until some interrupt is received
				p.start()
				p.stop()
			}

			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.config.BindPFlags(cmd.Flags())
		},
	}

	c.setAllFlags(cmd)
	c.root.AddCommand(cmd)
	return nil
}

type program struct {
	start func()
	stop  func()
}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	go p.start()
	return nil
}

func (p *program) Stop(s service.Service) error {
	p.stop()
	return nil
}

func (c *command) configureSigner(cmd *cobra.Command, logger logging.Logger) (config *crypto.SignerConfig, err error) {
	if c.config.GetString(optionNameDataDir) == "" {
		logger.Warning("data directory not provided, keys are not persisted")
		return nil, errors.New("data directory not provided")
	}
	path := filepath.Join(c.config.GetString(optionNameDataDir), "keys")
	p2pKT := p2pkey.New(path)
	subKT := subkey.New(path)

	var password string
	if p := c.config.GetString(optionNamePassword); p != "" {
		password = p
	} else if pf := c.config.GetString(optionNamePasswordFile); pf != "" {
		b, err := os.ReadFile(pf)
		if err != nil {
			return nil, err
		}
		password = string(bytes.Trim(b, "\n"))
	} else {
		// if libp2p key exists we can assume all required keys exist
		// so prompt for a password to unlock them
		// otherwise prompt for new password with confirmation to create them
		exists, err := p2pKT.Exists("libp2p")
		if err != nil {
			return nil, err
		}
		if exists {
			password, err = terminalPromptPassword(cmd, c.passwordReader, "Password")
			if err != nil {
				return nil, err
			}
		} else {
			password, err = terminalPromptCreatePassword(cmd, c.passwordReader)
			if err != nil {
				return nil, err
			}
		}
	}

	kp, created, err := subKT.Key("boson", password)
	if err != nil {
		return nil, fmt.Errorf("boson key: %w", err)
	}

	var addr boson.Address
	addr, err = crypto.NewOverlayAddress(kp.Public().Encode(), c.config.GetUint64(optionNameNetworkID))
	if err != nil {
		return nil, err
	}

	if created {
		logger.Infof("new boson network address created: %s", addr)
	} else {
		logger.Infof("using existing boson network address: %s", addr)
	}

	logger.Infof("boson public key %s", kp.Public().Hex())
	logger.Infof("boson ss58 address %s", kp.Public().Address())

	libp2pPrivateKey, created, err := p2pKT.Key("libp2p", password)
	if err != nil {
		return nil, fmt.Errorf("libp2p key: %w", err)
	}
	if created {
		logger.Debugf("new libp2p key created")
	} else {
		logger.Debugf("using existing libp2p key")
	}

	keyPair, err := signature.KeyringPairFromSecret(fmt.Sprintf("0x%x", kp.GetSeed()), 42)
	if err != nil {
		return nil, fmt.Errorf("subkey keyPair: %w", err)
	}
	return &crypto.SignerConfig{
		Signer:           kp,
		Overlay:          addr,
		Libp2pPrivateKey: libp2pPrivateKey,
		SubKey:           keyPair,
	}, nil
}
