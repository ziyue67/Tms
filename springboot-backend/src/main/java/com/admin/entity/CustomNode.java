package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

/** An externally hosted client node imported by an administrator. */
@Data
@TableName("custom_node")
public class CustomNode {
    private Long id;
    private String name;
    private String protocol;
    private String rawLink;
    private String parsedJson;
    /** global/subscribers = aggregate scope; users = explicit assignment scope. */
    private String visibility;
    private Integer status;
    private Long createdTime;
    private Long updatedTime;
}
