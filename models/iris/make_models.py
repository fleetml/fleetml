"""Builds the tiny Iris classifiers used by the Android sample app and the demo.

    python3 -m venv .venv && .venv/bin/pip install 'tensorflow==2.17.*' 'numpy<2' scikit-learn
    .venv/bin/python models/iris/make_models.py

Writes iris_v1.tflite (fewer training epochs) and iris_v2.tflite next to this
file. Input: float32[1, 4] (sepal length, sepal width, petal length, petal
width in cm). Output: float32[1, 3] class probabilities (setosa, versicolor,
virginica). Training data: the Iris dataset (R. A. Fisher, 1936) as shipped
with scikit-learn.
"""
import os

import numpy as np
import tensorflow as tf
from sklearn.datasets import load_iris

HERE = os.path.dirname(os.path.abspath(__file__))


def build(epochs: int, seed: int) -> bytes:
    tf.keras.utils.set_random_seed(seed)
    x, y = load_iris(return_X_y=True)
    x = x.astype(np.float32)
    norm = tf.keras.layers.Normalization()
    norm.adapt(x)
    model = tf.keras.Sequential([
        tf.keras.Input(shape=(4,), batch_size=1, name="measurements"),
        norm,
        tf.keras.layers.Dense(8, activation="relu"),
        tf.keras.layers.Dense(3, activation="softmax"),
    ])
    model.compile(optimizer=tf.keras.optimizers.Adam(0.05), loss="sparse_categorical_crossentropy", metrics=["accuracy"])
    model.fit(x, y, epochs=epochs, batch_size=16, verbose=0)
    _, acc = model.evaluate(x, y, verbose=0)
    print(f"epochs={epochs} training accuracy={acc:.3f}")
    return tf.lite.TFLiteConverter.from_keras_model(model).convert()


for name, epochs, seed in [("iris_v1.tflite", 3, 1), ("iris_v2.tflite", 60, 2)]:
    with open(os.path.join(HERE, name), "wb") as f:
        f.write(build(epochs, seed))
