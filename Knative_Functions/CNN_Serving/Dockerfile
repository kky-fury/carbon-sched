FROM python:3.6-slim

RUN pip install --upgrade --no-cache-dir pip
RUN pip install --no-cache-dir tensorflow==1.4.0 keras==2.2.4 h5py protobuf grpcio grpcio-tools grpcio-reflection Pillow

WORKDIR /app
COPY faasfunc_server.py faasfunc_pb2.py faasfunc_pb2_grpc.py image.jpg image2.jpg squeezenet.py  ./
COPY squeezenet_weights_tf_dim_ordering_tf_kernels.h5 /tmp

ENV PORT 50051
CMD python faasfunc_server.py -p $PORT
