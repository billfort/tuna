package main

import (
	"log"

	"github.com/nknorg/nkn-sdk-go"
	"github.com/nknorg/nkn/common"
	"github.com/nknorg/tuna"
)

type ExitCommand struct {
	ConfigFile string `short:"c" long:"config" description:"Config file path" default:"config.exit.json"`
}

var exitCommand ExitCommand

func (e *ExitCommand) Execute(args []string) error {
	config := &tuna.ExitConfiguration{
		SubscriptionPrefix:        tuna.DefaultSubscriptionPrefix,
		ReverseSubscriptionPrefix: tuna.DefaultSubscriptionPrefix,
	}
	err := tuna.ReadJSON(e.ConfigFile, config)
	if err != nil {
		log.Fatalln("Load config file error:", err)
	}
	if len(opts.BeneficiaryAddr) > 0 {
		config.BeneficiaryAddr = opts.BeneficiaryAddr
	}

	if len(config.BeneficiaryAddr) > 0 {
		_, err = common.ToScriptHash(config.BeneficiaryAddr)
		if err != nil {
			log.Fatalln("Invalid beneficiary address:", err)
		}
	}

	account, err := tuna.LoadOrCreateAccount(opts.WalletFile, opts.PasswordFile)
	if err != nil {
		log.Fatalln("Load or create account error:", err)
	}

	wallet, err := nkn.NewWallet(&nkn.Account{account}, nil)
	if err != nil {
		log.Fatalln("Create wallet error:", err)
	}

	log.Println("Your NKN wallet address is:", wallet.Address())

	var services []tuna.Service
	err = tuna.ReadJSON(opts.ServicesFile, &services)
	if err != nil {
		log.Fatalln("Load service file error:", err)
	}

	if config.Reverse {
		for _, service := range services {
			if _, ok := config.Services[service.Name]; ok {
				te, err := tuna.NewTunaExit(config, []tuna.Service{service}, wallet)
				if err != nil {
					log.Fatalln(err)
				}

				go func() {
					defer te.Close()
					err := te.StartReverse(true)
					if err != nil {
						log.Fatalln(err)
					}
				}()

				go func(service tuna.Service) {
					for range te.OnConnect.C {
						log.Printf("Service: %s, Address: %v:%v\n", service.Name, te.GetReverseIP(), te.GetReverseTCPPorts())
					}
				}(service)
			}
		}
	} else {
		te, err := tuna.NewTunaExit(config, services, wallet)
		if err != nil {
			log.Fatalln(err)
		}

		err = te.Start()
		if err != nil {
			log.Fatalln(err)
		}

		defer te.Close()
	}

	select {}
}

func init() {
	parser.AddCommand("exit", "Tuna exit mode", "Start tuna in exit mode", &exitCommand)
}