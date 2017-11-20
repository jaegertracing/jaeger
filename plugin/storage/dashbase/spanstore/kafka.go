package dashbase
import (
	"github.com/Shopify/sarama"
)

var producer sarama.AsyncProducer

func OpenKafka(hosts []string) error {
	config := sarama.NewConfig()
	producer2, err := sarama.NewAsyncProducer(hosts, config)
	if err != nil {
		return err
	}
	producer = producer2
	return nil
}

func PublishKafka(topic string, payload []byte) {
	message := &sarama.ProducerMessage{Topic: topic, Value: sarama.ByteEncoder(payload)}
	producer.Input() <- message
}

func CloseKafka() {
	producer.Close()
}
