#!/bin/bash

for type in A AAAA MX NS TXT SOA CNAME; do \
   echo "\n--- Записи типа $type ---"; \
   dig @192.168.88.88 yandex.ru $type +noall +answer; \
done