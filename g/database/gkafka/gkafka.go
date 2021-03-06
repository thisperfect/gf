// Copyright 2017 gf Author(https://gitee.com/johng/gf). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://gitee.com/johng/gf.

// Kafka客户端.
package gkafka

import (
    "time"
    "strings"
    "github.com/Shopify/sarama"
    "github.com/bsm/sarama-cluster"
    "gitee.com/johng/gf/g/os/glog"
)

// kafka Client based on sarama.Config
type Config struct {
    GroupId  string // group id for consumer.
    Servers  string // server list, multiple servers joined by ','.
    Topics   string // topic list, multiple topics joined by ','.
    sarama.Config
}

// Kafka Client(Consumer/SyncProducer/AsyncProducer)
type Client struct {
    Config        *Config
    consumer      *cluster.Consumer
    syncProducer  sarama.SyncProducer
    asyncProducer sarama.AsyncProducer
}

// Kafka Message.
type Message struct {
    Value          []byte
    Key            []byte
    Topic          string
    Partition      int
    Offset         int
}


// New a kafka client.
func New(config Config) *Client {
    config.Config = *sarama.NewConfig()

    // default config for consumer
    config.Consumer.Return.Errors = true
    if config.Consumer.Offsets.Initial == 0 {
        config.Consumer.Offsets.Initial = sarama.OffsetOldest
    }
    if config.Consumer.Offsets.CommitInterval == 0 {
        config.Consumer.Offsets.CommitInterval = 1 * time.Second
    }

    // default config for producer
    config.Producer.Return.Errors    = true
    config.Producer.Return.Successes = true
    if config.Producer.Timeout == 0 {
        config.Producer.Timeout = 5 * time.Second
    }

    return &Client {
        Config : &config,
    }
}

// Close client.
func (client *Client) Close() {
    if client.consumer != nil {
        client.consumer.Close()
    }
    if client.syncProducer != nil {
        client.syncProducer.Close()
    }
    if client.asyncProducer != nil {
        client.asyncProducer.Close()
    }
}

// Receive message from kafka from specified topics in config, in BLOCKING way, gkafka will handle offset tracking automatically.
func (client *Client) Receive() (*Message, error) {
    config       := cluster.NewConfig()
    config.Config = client.Config.Config
    config.Group.Return.Notifications = true
    if client.consumer == nil {
        c, err := cluster.NewConsumer(strings.Split(client.Config.Servers, ","), client.Config.GroupId, strings.Split(client.Config.Topics, ","), config)
        if err != nil {
            return nil, err
        } else {
            client.consumer = c
            go func(c *cluster.Consumer) {
                errors := c.Errors()
                notify := c.Notifications()
                for {
                    select {
                        case err := <-errors:
                            glog.Error(err)
                        case <-notify:
                    }
                }
            }(client.consumer)
        }
    }

    msg := <- client.consumer.Messages()
    client.consumer.MarkOffset(msg, "")
    return &Message {
        Value     : msg.Value,
        Key       : msg.Key,
        Topic     : msg.Topic,
        Partition : int(msg.Partition),
        Offset    : int(msg.Offset),
    }, nil
}

// Send data to kafka in synchronized way.
func (client *Client) SyncSend(message *Message) error {
    if client.syncProducer == nil {
        if p, err := sarama.NewSyncProducer(strings.Split(client.Config.Servers, ","), &client.Config.Config); err != nil {
            return err
        } else {
            client.syncProducer = p
        }
    }
    for _, topic := range strings.Split(client.Config.Topics, ",") {
        msg := messageToProducerMessage(message)
        msg.Topic = topic
        if _, _, err := client.syncProducer.SendMessage(msg); err != nil {
            return err
        }
    }
    return nil
}

// Send data to kafka in asynchronized way.
func (client *Client) AsyncSend(message *Message) error {
    if client.asyncProducer == nil {
        if p, err := sarama.NewAsyncProducer(strings.Split(client.Config.Servers, ","), &client.Config.Config); err != nil {
            return err
        } else {
            client.asyncProducer = p
            go func(p sarama.AsyncProducer) {
                errors  := p.Errors()
                success := p.Successes()
                for {
                    select {
                        case err := <-errors:
                            glog.Error(err)
                        case <-success:
                    }
                }
            }(client.asyncProducer)
        }
    }

    for _, topic := range strings.Split(client.Config.Topics, ",") {
        msg := messageToProducerMessage(message)
        msg.Topic = topic
        client.asyncProducer.Input() <- msg
    }
    return nil
}

// Convert *gkafka.Message to *sarama.ProducerMessage
func messageToProducerMessage(message *Message) *sarama.ProducerMessage {
    return &sarama.ProducerMessage {
        Topic     : message.Topic,
        Key       : sarama.ByteEncoder(message.Key),
        Value     : sarama.ByteEncoder(message.Value),
        Partition : int32(message.Partition),
        Offset    : int64(message.Offset),
    }
}
