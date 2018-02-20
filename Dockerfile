FROM scratch
ADD ./redis-template /
CMD ["/redis-template"]