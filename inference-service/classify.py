import tensorflow as tf
import numpy as np
from tensorflow.keras.applications import mobilenet_v2
from tensorflow.keras.applications.mobilenet_v2 import decode_predictions
from PIL import Image
import io

def classify_image(image_bytes: bytes) -> str:
    # Load image from bytes
    img = Image.open(io.BytesIO(image_bytes)).convert("RGB")

    # Resize to 224x224 as required by MobileNetV2
    img = img.resize((224, 224))
    img_array = tf.keras.preprocessing.image.img_to_array(img)

    # Add batch dimension and preprocess for MobileNetV2
    img_array = np.expand_dims(img_array, axis=0)
    img_array = mobilenet_v2.preprocess_input(img_array)

    # Load pre-trained model (only once, if needed)
    if not hasattr(classify_image, "model"):
        classify_image.model = mobilenet_v2.MobileNetV2(weights="imagenet")

    # Predict and decode the top label
    preds = classify_image.model.predict(img_array)
    decoded = decode_predictions(preds, top=1)[0][0]  # (class_id, label, confidence)
    _, label, confidence = decoded

    return f"{label} ({confidence:.2f})"
