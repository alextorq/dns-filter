#!/bin/bash

for type in A AAAA MX NS TXT SOA CNAME; do \
   echo "\n--- Записи типа $type ---"; \
   dig @127.0.0.1 google.com $type +noall +answer; \
done