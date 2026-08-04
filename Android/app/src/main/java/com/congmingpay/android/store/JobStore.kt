package com.congmingpay.android.store

import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper
import com.congmingpay.android.logger.Logger
import com.congmingpay.android.model.Printer
import com.google.gson.Gson
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Date
import java.util.Locale

/**
 * 任务持久化：原生 SQLite（jobs.db），对齐 Windows modernc/sqlite。
 * 不再使用 Room；调用方仍应在后台线程访问（避免主线程卡顿）。
 */
class JobStore private constructor(private val db: SQLiteDatabase) {

    private val gson = Gson()
    private val rfc3339 = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US)

    private fun nowRfc3339(): String = rfc3339.format(Date())

    fun getNextNo(): Int {
        db.rawQuery("SELECT value FROM meta WHERE key = ?", arrayOf(KEY_NEXT_NO)).use { c ->
            if (!c.moveToFirst()) return 1000
            val n = c.getString(0)?.toIntOrNull() ?: 1000
            return if (n < 1000) 1000 else n
        }
    }

    fun setNextNo(n: Int) {
        val cv = ContentValues().apply {
            put("key", KEY_NEXT_NO)
            put("value", n.toString())
        }
        db.insertWithOnConflict("meta", null, cv, SQLiteDatabase.CONFLICT_REPLACE)
    }

    fun upsertJob(job: PersistedJob) {
        val cv = job.toContentValues(gson, nowRfc3339())
        db.insertWithOnConflict("jobs", null, cv, SQLiteDatabase.CONFLICT_REPLACE)
    }

    fun deleteJob(no: Int) {
        db.delete("jobs", "no = ?", arrayOf(no.toString()))
    }

    fun deleteDone() {
        db.delete("jobs", "status IN ('done','failed')", null)
    }

    fun pruneTerminal(historyDays: Int): Int {
        val cal = Calendar.getInstance()
        cal.add(Calendar.DAY_OF_YEAR, -historyDays)
        val cutoff = rfc3339.format(cal.time)
        return db.delete("jobs", "status IN ('done','failed') AND createdAt < ?", arrayOf(cutoff))
    }

    fun loadActive(): List<PersistedJob> {
        db.rawQuery(
            "SELECT * FROM jobs WHERE status IN ('queued','printing','waiting')",
            null
        ).use { c ->
            val out = ArrayList<PersistedJob>(c.count)
            while (c.moveToNext()) out.add(c.toPersisted(gson))
            return out
        }
    }

    fun loadByNo(no: Int): PersistedJob? {
        db.rawQuery("SELECT * FROM jobs WHERE no = ?", arrayOf(no.toString())).use { c ->
            if (!c.moveToFirst()) return null
            return c.toPersisted(gson)
        }
    }

    fun loadAllList(): List<JobListRow> {
        db.rawQuery(
            "SELECT no, doc, status, timeLabel, err, printerJson FROM jobs ORDER BY no DESC",
            null
        ).use { c ->
            val out = ArrayList<JobListRow>(c.count)
            while (c.moveToNext()) {
                val printerJson = c.getString(c.getColumnIndexOrThrow("printerJson")) ?: ""
                val name = try {
                    gson.fromJson(printerJson, Printer::class.java)?.name ?: ""
                } catch (_: Exception) {
                    ""
                }
                out.add(
                    JobListRow(
                        no = c.getInt(c.getColumnIndexOrThrow("no")),
                        doc = c.getString(c.getColumnIndexOrThrow("doc")) ?: "",
                        status = c.getString(c.getColumnIndexOrThrow("status")) ?: "",
                        timeLabel = c.getString(c.getColumnIndexOrThrow("timeLabel")) ?: "",
                        err = c.getString(c.getColumnIndexOrThrow("err")) ?: "",
                        printerName = name
                    )
                )
            }
            return out
        }
    }

    companion object {
        private const val KEY_NEXT_NO = "next_no"

        @Volatile
        private var instance: JobStore? = null

        /** 打开（或复用）jobs.db；文件位于应用 databases 目录。 */
        fun open(context: Context): JobStore {
            instance?.let { return it }
            synchronized(this) {
                instance?.let { return it }
                val helper = JobsDbHelper(context.applicationContext)
                val store = JobStore(helper.writableDatabase)
                instance = store
                Logger.info("JobStore: SQLite jobs.db 已打开")
                return store
            }
        }
    }
}

/** 队列列表行（投影，无 BLOB）。 */
data class JobListRow(
    val no: Int,
    val doc: String,
    val status: String,
    val timeLabel: String,
    val err: String,
    val printerName: String
)

/** 持久化任务数据（与 DB 行对应）。 */
data class PersistedJob(
    val no: Int,
    val doc: String,
    val status: String,
    val timeLabel: String = "",
    val err: String = "",
    val printer: Printer,
    val data: ByteArray,
    val cut: Boolean,
    val buzzer: Boolean,
    val headLines: Int,
    val tailLines: Int,
    val reprintNext: Boolean,
    val cloudId: Long?,
    val contentType: Int = -1,
    val sourceJson: ByteArray? = null
)

private fun PersistedJob.toContentValues(gson: Gson, createdAt: String): ContentValues {
    return ContentValues().apply {
        put("no", no)
        put("doc", doc)
        put("status", status)
        put("timeLabel", timeLabel)
        put("err", err)
        put("createdAt", createdAt)
        put("printerJson", gson.toJson(printer))
        put("dataBlob", data)
        put("cut", if (cut) 1 else 0)
        put("buzzer", if (buzzer) 1 else 0)
        put("headLines", headLines)
        put("tailLines", tailLines)
        put("reprintNext", if (reprintNext) 1 else 0)
        if (cloudId != null) put("cloudId", cloudId) else putNull("cloudId")
        put("contentType", contentType)
        if (sourceJson != null) put("sourceJson", sourceJson) else putNull("sourceJson")
    }
}

private fun Cursor.toPersisted(gson: Gson): PersistedJob {
    val printerJson = getString(getColumnIndexOrThrow("printerJson")) ?: "{}"
    val cloudIdx = getColumnIndexOrThrow("cloudId")
    val srcIdx = getColumnIndexOrThrow("sourceJson")
    return PersistedJob(
        no = getInt(getColumnIndexOrThrow("no")),
        doc = getString(getColumnIndexOrThrow("doc")) ?: "",
        status = getString(getColumnIndexOrThrow("status")) ?: "",
        timeLabel = getString(getColumnIndexOrThrow("timeLabel")) ?: "",
        err = getString(getColumnIndexOrThrow("err")) ?: "",
        printer = try {
            gson.fromJson(printerJson, Printer::class.java) ?: Printer()
        } catch (_: Exception) {
            Printer()
        },
        data = getBlob(getColumnIndexOrThrow("dataBlob")) ?: ByteArray(0),
        cut = getInt(getColumnIndexOrThrow("cut")) != 0,
        buzzer = getInt(getColumnIndexOrThrow("buzzer")) != 0,
        headLines = getInt(getColumnIndexOrThrow("headLines")),
        tailLines = getInt(getColumnIndexOrThrow("tailLines")),
        reprintNext = getInt(getColumnIndexOrThrow("reprintNext")) != 0,
        cloudId = if (isNull(cloudIdx)) null else getLong(cloudIdx),
        contentType = getInt(getColumnIndexOrThrow("contentType")),
        sourceJson = if (isNull(srcIdx)) null else getBlob(srcIdx)
    )
}

/**
 * SQLiteOpenHelper：表结构与原 Room jobs/meta 一致，可继续使用已有 jobs.db。
 */
private class JobsDbHelper(context: Context) : SQLiteOpenHelper(context, DB_NAME, null, DB_VERSION) {

    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            """
            CREATE TABLE IF NOT EXISTS jobs (
              no INTEGER PRIMARY KEY NOT NULL,
              doc TEXT NOT NULL,
              status TEXT NOT NULL,
              timeLabel TEXT NOT NULL,
              err TEXT NOT NULL,
              createdAt TEXT NOT NULL,
              printerJson TEXT NOT NULL,
              dataBlob BLOB NOT NULL,
              cut INTEGER NOT NULL,
              buzzer INTEGER NOT NULL,
              headLines INTEGER NOT NULL,
              tailLines INTEGER NOT NULL,
              reprintNext INTEGER NOT NULL,
              cloudId INTEGER,
              contentType INTEGER NOT NULL,
              sourceJson BLOB
            )
            """.trimIndent()
        )
        db.execSQL(
            """
            CREATE TABLE IF NOT EXISTS meta (
              key TEXT PRIMARY KEY NOT NULL,
              value TEXT NOT NULL
            )
            """.trimIndent()
        )
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
        // 开发期：结构变更时重建（与 Room fallbackToDestructiveMigration 一致）
        db.execSQL("DROP TABLE IF EXISTS jobs")
        db.execSQL("DROP TABLE IF EXISTS meta")
        onCreate(db)
    }

    companion object {
        const val DB_NAME = "jobs.db"
        const val DB_VERSION = 1
    }
}
