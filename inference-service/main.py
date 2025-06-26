import asyncio
import os
import json
import random
import logging
from dotenv import load_dotenv
import nats
from nats.errors import TimeoutError
from nats.js.api import StreamConfig, RetentionPolicy, ConsumerConfig, ObjectStoreConfig, AckPolicy, DeliverPolicy

logging.basicConfig(level=logging.INFO)

BUCKET_NAME = "images"
STREAM_NAME = "INFERENCE_EVENTS"
START_CLASSIFY_SUBJECT = "images.classify"
NEW_LABEL_SUBJECT = "images.metadata.label"
START_CLASSIFY_NAME = "new-label-consumer"

class NewLabelMessage:
    def __init__(self, image_id, label):
        self.image_id = image_id
        self.label = label

    def to_json(self):
        return json.dumps({
            "image_id": self.image_id,
            "label": self.label
        })


async def main():
    # Load .env
    load_dotenv()
    nats_url = os.getenv("NATS_URL")
    if not nats_url:
        raise RuntimeError("NATS_URL not found in environment")

    nc = await nats.connect(nats_url)
    logging.info(f"Connected to NATS: {nats_url}")

    js = nc.jetstream()

    # Create object store
    try:
        store = await js.create_object_store(BUCKET_NAME)
    except Exception:
        store = await js.object_store(BUCKET_NAME)

    # Create stream
    try:
        await js.add_stream(StreamConfig(
            name=STREAM_NAME,
            subjects=[START_CLASSIFY_SUBJECT],
            retention=RetentionPolicy.WORK_QUEUE
        ))
    except Exception:
        pass  # Stream already exists

    async def message_handler(msg):
        image_id = msg.data.decode()
        try:
            obj = await store.get(image_id)
            image_bytes = obj.data
        except Exception as e:
            logging.warning(f"could not get image from object store: {e}")
            await msg.ack()
            return

        logging.info(f"Received image for classification: {image_id}, size: {len(image_bytes)} bytes")

        # Simulate classification
        new_label = str(random.randint(0, 9999999))
        label_msg = NewLabelMessage(image_id=image_id, label=new_label)

        await js.publish(NEW_LABEL_SUBJECT, label_msg.to_json().encode())
        await msg.ack()

    # Create consumer and subscribe to messages
    consumer_config = ConsumerConfig(
        name=START_CLASSIFY_NAME,
        durable_name=START_CLASSIFY_NAME,
        ack_policy=AckPolicy.EXPLICIT,
        deliver_policy=DeliverPolicy.ALL,
        ack_wait=300  # 5 minutes
    )
    sub = await js.pull_subscribe(subject=START_CLASSIFY_SUBJECT, durable=START_CLASSIFY_NAME, config=consumer_config, stream=STREAM_NAME)
    try:
        while True:
            try:
                msgs = await sub.fetch()
                for msg in msgs:
                    await message_handler(msg)
            except nats.errors.TimeoutError:
                await asyncio.sleep(1)
    except KeyboardInterrupt:
        pass

    await nc.drain()


if __name__ == "__main__":
    asyncio.run(main())
