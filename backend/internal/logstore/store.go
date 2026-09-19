package logstore

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

const (
	queueSize     = 10000
	batchSize     = 200
	flushInterval = time.Second

	esperaInicial  = time.Second
	esperaTeto     = 30 * time.Second
	maxTentativas  = 6
	intervaloAviso = time.Minute
)

var std = sync.OnceValue(func() *writer { return newWriter(queueSize, insertEntries) })

func Save(serverID, source, container, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	std().save(database.LogEntry{
		ServerID:  serverID,
		Source:    source,
		Container: container,
		Line:      line,
		Timestamp: time.Now().UTC(),
	})
}

func Flush() {
	std().flush()
}

type writer struct {
	queue    chan database.LogEntry
	flushReq chan chan struct{}
	insert   func([]database.LogEntry) error
	lost     atomic.Int64
	reported atomic.Int64

	conexaoCaiu   func(error) bool
	espera        func(time.Duration)
	maxTentativas int
	ultimoAviso   time.Time
}

func newWriter(size int, insert func([]database.LogEntry) error) *writer {
	w := &writer{
		queue:         make(chan database.LogEntry, size),
		flushReq:      make(chan chan struct{}),
		insert:        insert,
		conexaoCaiu:   bancoForaDoAr,
		espera:        time.Sleep,
		maxTentativas: maxTentativas,
	}
	safego.Run(context.Background(), "logstore:writer", func(context.Context) { w.run() })
	return w
}

func (w *writer) save(e database.LogEntry) {
	select {
	case w.queue <- e:
	default:
		w.lost.Add(1)
		observabilidade.LogsDescartados.Add(1)
	}
}

func (w *writer) dropped() int64 {
	return w.lost.Load()
}

func (w *writer) flush() {
	done := make(chan struct{})
	w.flushReq <- done
	<-done
}

func (w *writer) run() {
	buf := make([]database.LogEntry, 0, batchSize)
	var deadline <-chan time.Time

	write := func() {
		if len(buf) > 0 {
			w.write(buf)
			buf = buf[:0]
		}
		deadline = nil
		w.reportDropped()
	}
	add := func(e database.LogEntry) {
		if len(buf) == 0 {
			deadline = time.After(flushInterval)
		}
		buf = append(buf, e)
		if len(buf) >= batchSize {
			write()
		}
	}

	for {
		select {
		case e := <-w.queue:
			add(e)
		case <-deadline:
			write()
		case done := <-w.flushReq:
			for drained := false; !drained; {
				select {
				case e := <-w.queue:
					add(e)
				default:
					drained = true
				}
			}
			write()
			close(done)
		}
	}
}

func (w *writer) write(batch []database.LogEntry) {
	err := w.insert(batch)
	if err == nil {
		return
	}
	if errors.Is(err, errSemBanco) {
		w.lost.Add(int64(len(batch)))
		w.avisar("[LogStore] banco não conectado: %d linhas de log descartadas", len(batch))
		return
	}
	if w.conexaoCaiu(err) {
		w.reenviar(batch, err)
		return
	}
	failed := 0
	for i := range batch {
		if err := w.insert(batch[i : i+1]); err != nil {
			failed++
			log.Printf("[LogStore] erro ao salvar log (source=%s container=%s): %v",
				batch[i].Source, batch[i].Container, err)
		}
	}
	if failed > 0 {
		log.Printf("[LogStore] %d de %d linhas do lote recusadas pelo banco", failed, len(batch))
	}
}

func (w *writer) reenviar(batch []database.LogEntry, causa error) {
	espera := esperaInicial
	for tentativa := 1; tentativa < w.maxTentativas; tentativa++ {
		w.espera(espera)
		if err := w.insert(batch); err == nil {
			w.avisar("[LogStore] banco de volta: %d linhas gravadas depois de %d tentativa(s)", len(batch), tentativa)
			return
		} else if !w.conexaoCaiu(err) {
			w.write(batch)
			return
		} else {
			causa = err
		}
		if espera < esperaTeto {
			espera *= 2
			if espera > esperaTeto {
				espera = esperaTeto
			}
		}
	}
	w.lost.Add(int64(len(batch)))
	w.avisar("[LogStore] banco fora do ar: %d linhas de log descartadas depois de %d tentativas (%v)",
		len(batch), w.maxTentativas, causa)
}

func (w *writer) avisar(formato string, args ...any) {
	agora := time.Now()
	if agora.Sub(w.ultimoAviso) < intervaloAviso {
		return
	}
	w.ultimoAviso = agora
	log.Printf(formato, args...)
}

func bancoForaDoAr(error) bool {
	if database.DB == nil {
		return true
	}
	sql, err := database.DB.DB()
	if err != nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return sql.PingContext(ctx) != nil
}

func Descartadas() int64 {
	return std().dropped()
}

func (w *writer) reportDropped() {
	total := w.lost.Load()
	if prev := w.reported.Swap(total); total > prev {
		log.Printf("[LogStore] fila cheia: %d linhas de log descartadas para não travar o stream", total-prev)
	}
}

var errSemBanco = errors.New("banco não conectado")

func insertEntries(entries []database.LogEntry) error {
	if database.DB == nil {
		return errSemBanco
	}
	return database.DB.Create(&entries).Error
}

func StartRetention(maxAge, interval time.Duration) {
	safego.Run(context.Background(), "logstore:retencao", func(context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			cutoff := time.Now().UTC().Add(-maxAge)
			n, err := database.PruneOlderThan("log_entries", "timestamp", cutoff)
			if err != nil {
				log.Printf("[LogStore] erro ao podar log_entries: %v", err)
			} else if n > 0 {
				log.Printf("[LogStore] %d linhas de log antigas removidas", n)
			}
			<-ticker.C
		}
	})
}
