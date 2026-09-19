//go:build windows

package main

import (
	"context"
	"log"
	"os"

	"golang.org/x/sys/windows/svc"
)

const nomeDoServico = "dockkeeper-agent"

func executarComoServico() bool {
	ehServico, err := svc.IsWindowsService()
	if err != nil || !ehServico {
		return false
	}

	if err := carregarArquivoDeEnv(caminhoDoArquivoDeEnv(os.Getenv), os.Setenv); err != nil {
		log.Printf("[Agent] %v", err)
	}
	if err := svc.Run(nomeDoServico, servico{}); err != nil {
		log.Printf("[Agent] servico %s terminou com erro: %v", nomeDoServico, err)
		os.Exit(1)
	}
	return true
}

type servico struct{}

func (servico) Execute(_ []string, pedidos <-chan svc.ChangeRequest, estado chan<- svc.Status) (bool, uint32) {
	estado <- svc.Status{State: svc.StartPending}

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	fim := make(chan struct{})
	go func() {
		iniciar(ctx)
		close(fim)
	}()

	estado <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case pedido := <-pedidos:
			switch pedido.Cmd {
			case svc.Interrogate:
				estado <- pedido.CurrentStatus
			case svc.Stop, svc.Shutdown:
				estado <- svc.Status{State: svc.StopPending, WaitHint: 5000}
				cancelar()
				<-fim
				return false, 0
			}
		case <-fim:
			return false, 1
		}
	}
}
