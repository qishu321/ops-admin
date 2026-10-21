import test from 'node:test'
import assert from 'node:assert/strict'
import { formatKafkaCount, kafkaOffsetAtLeast, kafkaOffsetPercent, kafkaPayload } from '../src/views/assets/kafka-topic.js'
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
