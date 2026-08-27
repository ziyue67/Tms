package com.admin.common.typehandler;

import org.apache.ibatis.type.JdbcType;
import org.apache.ibatis.type.MappedJdbcTypes;
import org.apache.ibatis.type.MappedTypes;
import org.apache.ibatis.type.TypeHandler;

import java.lang.reflect.Constructor;
import java.lang.reflect.Method;
import java.sql.CallableStatement;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Types;
import java.util.Locale;

/** Stores JSON text correctly in PostgreSQL json/jsonb and MySQL JSON columns. */
@MappedTypes(String.class)
@MappedJdbcTypes({JdbcType.VARCHAR, JdbcType.LONGVARCHAR, JdbcType.OTHER})
public class JsonTextTypeHandler implements TypeHandler<String> {

    @Override
    public void setParameter(PreparedStatement ps, int i, String parameter, JdbcType jdbcType) throws SQLException {
        if (parameter == null) {
            ps.setNull(i, isPostgres(ps.getConnection()) ? Types.OTHER : Types.VARCHAR);
            return;
        }
        if (!isPostgres(ps.getConnection())) {
            ps.setString(i, parameter);
            return;
        }
        // PGobject is supplied by the PostgreSQL runtime driver. Reflection keeps
        // the backend buildable with MySQL-only classpaths as well.
        try {
            Class<?> type = Class.forName("org.postgresql.util.PGobject");
            Constructor<?> constructor = type.getConstructor();
            Object value = constructor.newInstance();
            Method setType = type.getMethod("setType", String.class);
            Method setValue = type.getMethod("setValue", String.class);
            setType.invoke(value, "jsonb");
            setValue.invoke(value, parameter);
            ps.setObject(i, value);
        } catch (ReflectiveOperationException | RuntimeException e) {
            throw new SQLException("PostgreSQL JSON type handler is unavailable", e);
        }
    }

    @Override
    public String getResult(ResultSet rs, String columnName) throws SQLException {
        return rs.getString(columnName);
    }

    @Override
    public String getResult(ResultSet rs, int columnIndex) throws SQLException {
        return rs.getString(columnIndex);
    }

    @Override
    public String getResult(CallableStatement cs, int columnIndex) throws SQLException {
        return cs.getString(columnIndex);
    }

    private boolean isPostgres(Connection connection) throws SQLException {
        String product = connection.getMetaData().getDatabaseProductName();
        return product != null && product.toLowerCase(Locale.ROOT).contains("postgres");
    }
}
