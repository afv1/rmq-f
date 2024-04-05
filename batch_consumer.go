package rmq

import (
	"context"
)

type BatchConsumer interface {
	Consume(batch Deliveries)
}

type BatchConsumerPro interface {
	Consume(ctx context.Context, sigChan chan Signal, batch Deliveries)
}

type BatchConsumerFunc func(Deliveries)

type BatchConsumerProFunc func(context.Context, chan Signal, Deliveries)

func (batchConsumerFunc BatchConsumerFunc) Consume(batch Deliveries) {
	batchConsumerFunc(batch)
}

func (batchConsumerProFunc BatchConsumerProFunc) Consume(ctx context.Context, sigChan chan Signal, batch Deliveries) {
	batchConsumerProFunc(ctx, sigChan, batch)
}
