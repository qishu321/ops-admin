import test from 'node:test'
import assert from 'node:assert/strict'
import { formatKafkaCount, kafkaByteSize, kafkaEncodingLabel, kafkaMessageId, kafkaOffsetAtLeast, kafkaOffsetPercent, kafkaPayload } from '../src/views/assets/kafka-topic.js'
test('Kafka offsets larger than JavaScript integers retain precision',()=>{
 assert.equal(formatKafkaCount('9007199254741010'),'9,007,199,254,741,010')
 assert.equal(kafkaOffsetAtLeast('9007199254741010','9007199254741009'),true)
 assert.equal(kafkaOffsetAtLeast('9007199254741009','9007199254741010'),false)
 assert.equal(kafkaOffsetPercent('9007199254741010','9007199254741010'),100)
 assert.equal(kafkaOffsetPercent('0','0'),0)
 assert.equal(formatKafkaCount('—'),'—')
})
test('Kafka tombstones, binary bytes and structured messages remain distinguishable',()=>{
 assert.equal(kafkaPayload({null:true,text:''}),'null')
 assert.equal(kafkaPayload({null:false,text:''}),'')
 assert.equal(kafkaPayload({encoding:'base64',text:'/w=='},true),'/w==')
 assert.equal(kafkaPayload({text:'{"x":1}',encoding:'utf8'},true),'{\n  "x": 1\n}')
})
test('Kafka message panel labels encoding, byte size and row identity',()=>{
 assert.equal(kafkaEncodingLabel('utf8'),'UTF-8')
 assert.equal(kafkaEncodingLabel('base64'),'Base64')
 assert.equal(kafkaEncodingLabel(''),'UTF-8')
 assert.equal(kafkaEncodingLabel('avro'),'avro')
 assert.equal(kafkaByteSize(0),'0 B')
 assert.equal(kafkaByteSize(672),'672 B')
 assert.equal(kafkaByteSize(2048),'2.0 KB')
 assert.equal(kafkaByteSize(5*1024*1024),'5.0 MB')
 assert.equal(kafkaByteSize(undefined),'—')
 assert.equal(kafkaByteSize(-1),'—')
 assert.equal(kafkaMessageId({partition:3,offset:'6495384'}),'3:6495384')
 assert.equal(kafkaMessageId(null),'0:')
})
