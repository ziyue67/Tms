package com.admin.controller;

import com.admin.common.annotation.RequireRole;
import com.admin.common.aop.LogAnnotation;
import com.admin.common.dto.LandingDto;
import com.admin.common.lang.R;
import com.admin.service.LandingService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.validation.annotation.Validated;
import org.springframework.web.bind.annotation.*;

import java.util.Map;

/**
 * <p>
 * 落地(中转出口)前端控制器。粘贴分享链接建落地,可复用分给多台前置机。
 * </p>
 *
 * @author QAQ
 * @since 2026-07-23
 */
@RestController
@RequestMapping("/api/v1/landing")
@CrossOrigin
public class LandingController extends BaseController {

    @Autowired
    private LandingService landingService;

    @LogAnnotation
    @RequireRole
    @PostMapping("/create")
    public R create(@Validated @RequestBody LandingDto dto) {
        return landingService.createLanding(dto);
    }

    @RequireRole
    @PostMapping("/list")
    public R list() {
        return landingService.getLandings();
    }

    @LogAnnotation
    @RequireRole
    @PostMapping("/rename")
    public R rename(@RequestBody Map<String, Object> body) {
        Object id = body.get("id");
        return landingService.renameLanding(id == null ? null : Long.valueOf(String.valueOf(id)),
                body.get("name") == null ? "" : String.valueOf(body.get("name")));
    }

    @LogAnnotation
    @RequireRole
    @PostMapping("/delete")
    public R delete(@RequestBody Map<String, Object> body) {
        return landingService.deleteLanding(Long.valueOf(String.valueOf(body.get("id"))));
    }

    /** 中转:经前置机测一条落地链接能不能通,回显出口 IP */
    @RequireRole
    @PostMapping("/test")
    public R test(@RequestBody Map<String, Object> body) {
        return landingService.testLanding(
                Long.valueOf(String.valueOf(body.get("nodeId"))),
                String.valueOf(body.get("link")));
    }
}
