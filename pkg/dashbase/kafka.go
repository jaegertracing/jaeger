package dashbase
import (
	"github.com/Shopify/sarama"
)


type KafkaClient struct {
	producer sarama.AsyncProducer
	Hosts []string
}


func (k *KafkaClient) Open() error {
	config := sarama.NewConfig()
	producer, err := sarama.NewAsyncProducer(k.Hosts, config)
	if err != nil {
		return err
	}
	k.producer = producer
	return nil
}

func (k *KafkaClient) Send(topic string, payload []byte) {
	message := &sarama.ProducerMessage{Topic: topic, Value: sarama.ByteEncoder(payload)}
	k.producer.Input() <- message
}

func (k *KafkaClient) Close() {
	k.producer.Close()
}

func NewKafkaClient(hosts []string) (KafkaClient, error) {
	client := KafkaClient{Hosts: hosts}
	err := client.Open()
	return client, err
}