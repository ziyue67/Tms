package com.admin.entity;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;
import lombok.Data;

import java.io.Serializable;

/**
 * A protocol group that should be assigned automatically whenever a user gains
 * a usable subscription. landing_id=0 represents a direct protocol group.
 */
@Data
@TableName("inbound_auto_provision")
public class InboundAutoProvision implements Serializable {

    private static final long serialVersionUID = 1L;

    @TableId(value = "id", type = IdType.AUTO)
    private Long id;
    private Long nodeId;
    private Long landingId;
    private Integer enabled;
    private Long createdTime;
    private Long updatedTime;
}
