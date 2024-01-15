package rmq

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type Delivery interface {
	Payload() string
	// SetPayload sets the new payload and move delivery to the Ready List.
	SetPayload(payload string) error
	// SetPayloadAndReject sets the new payload and move delivery to the Rejected List.
	SetPayloadAndReject(payload string) error

	Ack() error
	Reject() error
	Push() error
	Return() error
}

var (
	_ Delivery   = &redisDelivery{}
	_ WithHeader = &redisDelivery{}
)

type redisDelivery struct {
	ctx          context.Context
	payload      string
	clearPayload string
	header       http.Header
	readyKey     string
	unackedKey   string
	rejectedKey  string
	pushKey      string
	redisClient  RedisClient
	errChan      chan<- error
}

func (delivery *redisDelivery) Header() http.Header {
	return delivery.header
}

func (delivery *redisDelivery) String() string {
	return fmt.Sprintf("[%s %s]", delivery.clearPayload, delivery.unackedKey)
}

func (delivery *redisDelivery) Payload() string {
	return delivery.clearPayload
}

// SetPayload implements Delivery interface.
func (delivery *redisDelivery) SetPayload(payload string) error {
	return delivery.setPayload(payload, delivery.readyKey)
}

// SetPayloadAndReject implements Delivery interface.
func (delivery *redisDelivery) SetPayloadAndReject(payload string) error {
	return delivery.setPayload(payload, delivery.rejectedKey)
}

// blocking versions of the functions below with the following behavior:
// 1. return immediately if the operation succeeded or failed with ErrorNotFound
// 2. in case of other redis errors, send them to the errors chan and retry after a sleep
// 3. if redis errors occur after StopConsuming() has been called, ErrorConsumingStopped will be returned

func (delivery *redisDelivery) Ack() error {
	errorCount := 0
	for {
		count, err := delivery.redisClient.LRem(delivery.unackedKey, 1, delivery.payload)
		if err == nil { // no redis error
			if count == 0 {
				return ErrorNotFound
			}
			return nil
		}

		// redis error

		errorCount++

		select { // try to add error to channel, but don't block
		case delivery.errChan <- &DeliveryError{Delivery: delivery, RedisErr: err, Count: errorCount}:
		default:
		}

		if err := delivery.ctx.Err(); err != nil {
			return ErrorConsumingStopped
		}

		time.Sleep(time.Second)
	}
}

func (delivery *redisDelivery) Reject() error {
	return delivery.move(delivery.rejectedKey)
}

func (delivery *redisDelivery) Push() error {
	if delivery.pushKey == "" {
		return delivery.Reject() // fall back to rejecting
	}

	return delivery.move(delivery.pushKey)
}

func (delivery *redisDelivery) Return() error {
	return delivery.move(delivery.readyKey)
}

func (delivery *redisDelivery) move(key string) error {
	errorCount := 0
	for {
		_, err := delivery.redisClient.LPush(key, delivery.payload)
		if err == nil { // success
			break
		}
		// error

		errorCount++

		select { // try to add error to channel, but don't block
		case delivery.errChan <- &DeliveryError{Delivery: delivery, RedisErr: err, Count: errorCount}:
		default:
		}

		if err := delivery.ctx.Err(); err != nil {
			return ErrorConsumingStopped
		}

		time.Sleep(time.Second)
	}

	return delivery.Ack()
}

// setPayload copies original delivery, sets the payload,
// adds delivery to the target queue by queueKey and Ack the original delivery to forget it.
func (delivery *redisDelivery) setPayload(payload string, queueKey string) error {
	// Copy original delivery.
	newDelivery := *delivery

	// Update deliveries payload.
	newDelivery.payload = payload
	newDelivery.clearPayload = payload

	// Move delivery to the Rejected queue.
	errorCount := 0
	for {
		// Add new delivery to the Rejected list.
		_, err := newDelivery.redisClient.LPush(queueKey, newDelivery.Payload())
		if err == nil { // success
			break
		}
		// error

		errorCount++

		select { // try to add error to channel, but don't block
		case newDelivery.errChan <- &DeliveryError{Delivery: &newDelivery, RedisErr: err, Count: errorCount}:
		default:
		}

		if err = newDelivery.ctx.Err(); err != nil {
			return ErrorConsumingStopped
		}

		time.Sleep(time.Second)
	}

	// Ack original delivery to forget it.
	return delivery.Ack()
}

// lower level functions which don't retry but just return the first error
