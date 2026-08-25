package com.admin.entity;

import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

@Data
@TableName("user_custom_node")
public class UserCustomNode {
    private Long id;
    private Long userId;
    private Long customNodeId;
    private Integer status;
    private Long createdTime;
}
