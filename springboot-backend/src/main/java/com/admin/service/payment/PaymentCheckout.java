package com.admin.service.payment;

import java.util.LinkedHashMap;
import java.util.Map;

/** Data required by the client to continue an off-site payment. */
public final class PaymentCheckout {
    private final String type;
    private final String url;
    private final Map<String, String> fields;
    private final String message;

    public PaymentCheckout(String type, String url, Map<String, String> fields, String message) {
        this.type = type;
        this.url = url;
        this.fields = fields == null ? new LinkedHashMap<>() : new LinkedHashMap<>(fields);
        this.message = message;
    }

    public static PaymentCheckout redirect(String url, String message) {
        return new PaymentCheckout("redirect", url, null, message);
    }

    public static PaymentCheckout form(String url, Map<String, String> fields, String message) {
        return new PaymentCheckout("form", url, fields, message);
    }

    public static PaymentCheckout qr(String codeUrl, String message) {
        return new PaymentCheckout("qr", codeUrl, null, message);
    }

    public String getType() { return type; }
    public String getUrl() { return url; }
    public Map<String, String> getFields() { return fields; }
    public String getMessage() { return message; }
}
