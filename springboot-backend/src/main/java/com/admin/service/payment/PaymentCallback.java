package com.admin.service.payment;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;

/** Preserves both parsed request values and the exact raw body used by webhooks. */
public final class PaymentCallback {
    private final String rawBody;
    private final Map<String, String> values;
    private final Map<String, String> headers;

    public PaymentCallback(String rawBody, Map<String, String> values, Map<String, String> headers) {
        this.rawBody = rawBody == null ? "" : rawBody;
        this.values = values == null ? Collections.emptyMap() : new LinkedHashMap<>(values);
        this.headers = headers == null ? Collections.emptyMap() : new LinkedHashMap<>(headers);
    }

    public String getRawBody() { return rawBody; }
    public Map<String, String> getValues() { return values; }
    public String value(String key) { return values.get(key); }
    public String header(String key) {
        for (Map.Entry<String, String> entry : headers.entrySet()) {
            if (entry.getKey().equalsIgnoreCase(key)) return entry.getValue();
        }
        return null;
    }
}
